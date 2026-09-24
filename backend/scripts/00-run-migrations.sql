-- Init hook for the postgres image: runs scripts/run-migrations.sh.
--
-- Why a .sql file and not the .sh script itself: docker-entrypoint.sh
-- *executes* a *.sh file in /docker-entrypoint-initdb.d when it looks
-- executable and only sources it otherwise. On Docker Desktop (macOS) a
-- bind-mounted file shows up as executable regardless of its mode on the
-- host, but exec from that mount fails ("bad interpreter: Permission
-- denied") - and the entrypoint then just moves on, leaving an empty
-- database. A *.sql file is always fed to psql, never executed, so the
-- behaviour no longer depends on host file permissions; the script is
-- read by `sh`, which only needs read access.
--
-- Runs with ON_ERROR_STOP (set by the entrypoint): if the script fails,
-- the RAISE below aborts initialisation instead of silently continuing.
\! sh /opt/invest/run-migrations.sh
\if :SHELL_ERROR
DO $$ BEGIN RAISE EXCEPTION 'run-migrations.sh failed - see the log above'; END $$;
\endif
