#!/bin/sh
# Applies every service's SQL migrations, each file at most once.
#
# Why this file exists: the official postgres image only looks at *files*
# directly inside /docker-entrypoint-initdb.d (see docker-entrypoint.sh in
# docker-library/postgres) - if an entry there is a directory, it is
# silently skipped ("ignoring ..."). Mounting each service's migrations/
# folder as a subdirectory of /docker-entrypoint-initdb.d (as this repo
# used to do for price_updater) therefore never actually runs anything.
#
# The fix: mount each service's migrations/ folder under
# /docker-entrypoint-initdb.d/migrations/<NN>_<service>/ instead, and mount
# *this* script directly as a *.sh file in /docker-entrypoint-initdb.d. The
# entrypoint sources *.sh files it finds there, so this one runs and, in
# turn, applies every *.sql file it finds under migrations/, one service
# subdirectory at a time, in alphabetical order.
#
# Ordering matters when one service's migration references another's
# tables (e.g. portfolio's trades table has a foreign key into
# price_updater's securities table), hence the NN_ prefixes on the mount
# points in docker-compose.yml (10_price_updater, 20_users,
# 30_portfolio, ...): price_updater's tables must exist before
# portfolio's migration runs.
#
# State: applied files are recorded in the schema_migrations table
# (service, filename), and each file is applied in one transaction together
# with its record, so a failed file leaves no partial state and a rerun
# only applies what is new. The postgres image runs this only on first
# init (empty data dir); to apply new migrations to an existing database,
# run it by hand:
#
#   docker compose exec postgres sh /docker-entrypoint-initdb.d/00-run-migrations.sh
#
# (A database created before this tracking existed has an empty
# schema_migrations table, so the first manual run re-applies every file
# once - they are written to be idempotent - and records them.)
#
# Everything runs in a subshell: the postgres entrypoint *sources* this
# file (it is not executable), so `set`/`exit` here must not leak into it.
(
    set -eu

    ROOT="${MIGRATIONS_ROOT:-/docker-entrypoint-initdb.d/migrations}"

    if [ ! -d "$ROOT" ]; then
        echo "run-migrations.sh: $ROOT not found, nothing to do"
        exit 0
    fi

    run_psql() {
        psql -v ON_ERROR_STOP=1 --no-psqlrc --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" "$@"
    }

    run_psql -q <<'SQL'
CREATE TABLE IF NOT EXISTS schema_migrations (
    service     TEXT NOT NULL,
    filename    TEXT NOT NULL,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (service, filename)
);
SQL

    for svc_dir in "$ROOT"/*/; do
        [ -d "$svc_dir" ] || continue
        # "10_price_updater" -> "price_updater": the NN_ prefix only fixes
        # the order, so renumbering a mount must not re-apply its files.
        svc=$(basename "$svc_dir")
        svc=${svc#[0-9]*_}
        for f in "$svc_dir"*.sql; do
            [ -e "$f" ] || continue
            name=$(basename "$f")

            applied=$(run_psql -tA -v svc="$svc" -v file="$name" <<'SQL'
SELECT count(*) FROM schema_migrations WHERE service = :'svc' AND filename = :'file';
SQL
)
            if [ "$applied" != "0" ]; then
                echo "run-migrations.sh: $svc/$name already applied, skipping"
                continue
            fi

            echo "run-migrations.sh: applying $svc/$name"
            {
                cat "$f"
                printf '\n;\nINSERT INTO schema_migrations (service, filename) VALUES (:%s, :%s);\n' "'svc'" "'file'"
            } | run_psql --single-transaction -v svc="$svc" -v file="$name" -f -
        done
    done
)
