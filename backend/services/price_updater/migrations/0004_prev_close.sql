-- Official close of the previous trading day, straight from MOEX
-- (securities.PREVLEGALCLOSEPRICE, else PREVPRICE) on every poll. Used for
-- "change for the day" instead of deriving it from stored daily candles,
-- which may be incomplete while the history backfill is still running.
--
-- price_updater also applies this on startup (store.EnsureSchema), so an
-- existing database needs no manual step.
ALTER TABLE latest_prices ADD COLUMN IF NOT EXISTS prev_close NUMERIC;
