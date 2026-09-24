#!/bin/sh
# Applies every service's SQL migrations on first-time Postgres container
# init.
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
set -eu

ROOT="/docker-entrypoint-initdb.d/migrations"

if [ ! -d "$ROOT" ]; then
    echo "run-migrations.sh: $ROOT not found, nothing to do"
    exit 0
fi

for svc_dir in "$ROOT"/*/; do
    [ -d "$svc_dir" ] || continue
    svc=$(basename "$svc_dir")
    for f in "$svc_dir"*.sql; do
        [ -e "$f" ] || continue
        echo "run-migrations.sh: applying $svc/$(basename "$f")"
        psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" -f "$f"
    done
done
