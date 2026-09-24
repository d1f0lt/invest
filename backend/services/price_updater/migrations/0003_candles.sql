-- Stop keeping a per-minute time series of prices (MOEX itself keeps the
-- full history). From now on price_updater stores:
--
--   latest_prices        - one row per (secid, board): the current price,
--                          upserted every poll. Same name and columns as the
--                          old view, so portfolio/price_reader queries keep
--                          working unchanged.
--   candles              - OHLC candles for charts:
--                            interval '1h' - built live from the per-minute
--                                            polls; only each security's last
--                                            trading day is kept (day chart);
--                            interval '1d' - built live during the day, then
--                                            overwritten by MOEX's official
--                                            daily history every night; kept
--                                            forever (week/month/year/all charts).
--   candle_history_sync  - per board, the last date whose official daily
--                          history has been loaded (drives catch-up/backfill).
--
-- Existing price_snapshots data is folded into latest_prices and into
-- hourly candles for the last few days (the nightly job trims them to the
-- last trading day), then the table is dropped.

-- Guarded so the migration can be re-run safely once latest_prices is a table.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_views WHERE viewname = 'latest_prices' AND schemaname = current_schema()) THEN
        DROP VIEW latest_prices;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS latest_prices (
    secid             TEXT NOT NULL,
    board             TEXT NOT NULL,
    last_price        NUMERIC,
    open_price        NUMERIC,
    high_price        NUMERIC,
    low_price         NUMERIC,
    value_today       NUMERIC,
    volume_today      BIGINT,
    trading_status    TEXT,
    moex_update_time  TEXT,
    collected_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (secid, board)
);

CREATE TABLE IF NOT EXISTS candles (
    secid       TEXT NOT NULL,
    board       TEXT NOT NULL,
    interval    TEXT NOT NULL CHECK (interval IN ('1h', '1d')),
    -- Start of the candle. Hourly: start of the hour (Europe/Moscow); daily: 00:00
    -- Europe/Moscow of the trade date.
    start_at    TIMESTAMPTZ NOT NULL,
    open        NUMERIC NOT NULL,
    high        NUMERIC NOT NULL,
    low         NUMERIC NOT NULL,
    close       NUMERIC NOT NULL,
    -- Daily candles only (hourly candles are sampled from minute polls,
    -- which carry no per-hour volume).
    volume      BIGINT,
    value       NUMERIC,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (secid, board, interval, start_at)
);

-- Retention cleanup of hourly candles scans by time, not by security.
CREATE INDEX IF NOT EXISTS idx_candles_interval_start_at
    ON candles (interval, start_at);

CREATE TABLE IF NOT EXISTS candle_history_sync (
    board           TEXT PRIMARY KEY,
    synced_through  DATE NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $$
BEGIN
    IF to_regclass('price_snapshots') IS NOT NULL THEN
        INSERT INTO latest_prices (
            secid, board, last_price, open_price, high_price, low_price,
            value_today, volume_today, trading_status, moex_update_time, collected_at
        )
        -- Latest snapshot per security, but with the last *known* price:
        -- latest_prices keeps it across sessions (see storage.UpsertLatestPrices).
        SELECT DISTINCT ON (s.secid, s.board)
            s.secid, s.board,
            COALESCE(s.last_price, (
                SELECT p.last_price FROM price_snapshots p
                WHERE p.secid = s.secid AND p.board = s.board AND p.last_price IS NOT NULL
                ORDER BY p.collected_at DESC LIMIT 1
            )),
            s.open_price, s.high_price, s.low_price,
            s.value_today, s.volume_today, s.trading_status, s.moex_update_time, s.collected_at
        FROM price_snapshots s
        ORDER BY s.secid, s.board, s.collected_at DESC
        ON CONFLICT (secid, board) DO NOTHING;

        INSERT INTO candles (secid, board, interval, start_at, open, high, low, close)
        SELECT secid, board, '1h', hour_start,
               (array_agg(last_price ORDER BY collected_at))[1],
               max(last_price),
               min(last_price),
               (array_agg(last_price ORDER BY collected_at DESC))[1]
        FROM (
            SELECT secid, board, last_price, collected_at,
                   -- Moscow-local hours, same buckets as updater.buildRows
                   -- (independent of the session's TimeZone setting).
                   date_trunc('hour', collected_at AT TIME ZONE 'Europe/Moscow')
                       AT TIME ZONE 'Europe/Moscow' AS hour_start
            FROM price_snapshots
            WHERE last_price IS NOT NULL
              AND trading_status = 'T'
              AND collected_at > now() - interval '4 days'
        ) s
        GROUP BY secid, board, hour_start
        ON CONFLICT DO NOTHING;

        DROP TABLE price_snapshots;
    END IF;
END $$;
