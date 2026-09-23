-- Broker report import: everything a report contains, not only trades,
-- so P&L can include dividends, coupons, taxes and fees, and a report can
-- be uploaded again (or overlap another one) without duplicating rows.

-- НКД (accrued coupon interest) paid on a bond buy / received on a sell,
-- total for the trade. Not part of price: price stays the clean price, so
-- the НКД shows up as its own line in P&L (it is paid back by the next
-- coupon).
ALTER TABLE trades ADD COLUMN IF NOT EXISTS accrued_interest NUMERIC NOT NULL DEFAULT 0
    CHECK (accrued_interest >= 0);

-- Broker's id of the trade (e.g. "sber:<account>:trade:<deal no>").
-- NULL for trades entered by hand.
ALTER TABLE trades ADD COLUMN IF NOT EXISTS external_id TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS uq_trades_portfolio_external_id
    ON trades (portfolio_id, external_id) WHERE external_id IS NOT NULL;

-- Money movements that are not trades. amount is signed from the
-- account's point of view (> 0 in, < 0 out). Like trades, an immutable
-- ledger.
CREATE TABLE IF NOT EXISTS cash_operations (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    portfolio_id UUID NOT NULL REFERENCES portfolios (id) ON DELETE CASCADE,
    type         TEXT NOT NULL CHECK (type IN (
                     'deposit', 'withdrawal', 'dividend', 'coupon',
                     'redemption', 'tax', 'fee', 'other')),
    amount       NUMERIC NOT NULL CHECK (amount <> 0),
    currency     TEXT NOT NULL DEFAULT 'RUB',
    occurred_at  TIMESTAMPTZ NOT NULL,

    -- Instrument for dividend/coupon/redemption. Deliberately no FK to
    -- securities (unlike trades): a dividend can arrive for a security
    -- price_updater doesn't track, and losing the income row would
    -- understate P&L, while a trade on an unknown instrument can't be
    -- valued anyway.
    secid        TEXT,
    board        TEXT,

    description  TEXT NOT NULL DEFAULT '',
    external_id  TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_cash_operations_portfolio_external_id
    ON cash_operations (portfolio_id, external_id) WHERE external_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_cash_operations_portfolio_id_occurred_at
    ON cash_operations (portfolio_id, occurred_at);
