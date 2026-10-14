ALTER TABLE candles DROP CONSTRAINT IF EXISTS candles_interval_check;
ALTER TABLE candles ADD CONSTRAINT candles_interval_check CHECK (interval IN ('10m', '1h', '1d'));
