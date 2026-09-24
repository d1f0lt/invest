-- price_updater schema: reference data for MOEX securities and a time
-- series of price snapshots collected roughly once per minute.

CREATE TABLE IF NOT EXISTS securities (
    secid       TEXT NOT NULL,
    board       TEXT NOT NULL,
    short_name  TEXT,
    sec_name    TEXT,
    isin        TEXT,
    currency    TEXT,
    lot_size    BIGINT,
    decimals    INT,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (secid, board)
);

CREATE TABLE IF NOT EXISTS price_snapshots (
    id                BIGSERIAL PRIMARY KEY,
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
    collected_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_price_snapshots_secid_board_collected_at
    ON price_snapshots (secid, board, collected_at DESC);

CREATE INDEX IF NOT EXISTS idx_price_snapshots_collected_at
    ON price_snapshots (collected_at);

-- Convenience view: the most recent snapshot per (secid, board).
CREATE OR REPLACE VIEW latest_prices AS
SELECT DISTINCT ON (secid, board)
    secid, board, last_price, open_price, high_price, low_price,
    value_today, volume_today, trading_status, moex_update_time, collected_at
FROM price_snapshots
ORDER BY secid, board, collected_at DESC;
