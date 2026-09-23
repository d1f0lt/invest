-- portfolio schema: a user's portfolios and the trades (buys/sells) in
-- each of them.
--
-- Deliberate design choice: this table stores an immutable ledger of
-- trades, not a mutable "current quantity" per stock. Current holdings
-- (internal/pnl in the Go code) and realized/unrealized P&L - including
-- for positions that have since been fully sold - are both derived from
-- this ledger. A mutable holdings table would lose the trade history
-- needed to compute P&L for a position you no longer hold, and can't be
-- reconstructed once overwritten.
--
-- Requires price_updater's migration (securities table) to already have
-- run - see docker-compose.yml's migration mount ordering and
-- scripts/run-migrations.sh.

CREATE TABLE IF NOT EXISTS portfolios (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL,
    name        TEXT NOT NULL DEFAULT 'Основной',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- No foreign key to users.id: that table lives in a different service's
-- migrations (same Postgres instance today, but a deliberately loose
-- coupling - see the portfolio service README for the tradeoff). user_id
-- is trusted from the caller's verified JWT, not looked up here.
CREATE INDEX IF NOT EXISTS idx_portfolios_user_id ON portfolios (user_id);

DO $$ BEGIN
    CREATE TYPE trade_side AS ENUM ('buy', 'sell');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS trades (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    portfolio_id    UUID NOT NULL REFERENCES portfolios (id) ON DELETE CASCADE,

    -- secid/board identify the instrument exactly like price_updater's
    -- securities table does (MOEX doesn't hand out a single opaque
    -- "unique number" per instrument the way some other exchanges do -
    -- secid+board together are MOEX's unique key, e.g. SBER/TQBR). The FK
    -- below ties a trade to a security price_updater actually knows
    -- about, which also means you can join trades/holdings straight into
    -- price_updater's `latest_prices` view for current market value.
    secid           TEXT NOT NULL,
    board           TEXT NOT NULL,

    side            trade_side NOT NULL,
    quantity        NUMERIC NOT NULL CHECK (quantity > 0),
    price           NUMERIC NOT NULL CHECK (price >= 0),
    fee             NUMERIC NOT NULL DEFAULT 0 CHECK (fee >= 0),
    currency        TEXT NOT NULL DEFAULT 'RUB',
    executed_at     TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    FOREIGN KEY (secid, board) REFERENCES securities (secid, board)
);

CREATE INDEX IF NOT EXISTS idx_trades_portfolio_id_executed_at
    ON trades (portfolio_id, executed_at);

CREATE INDEX IF NOT EXISTS idx_trades_secid_board ON trades (secid, board);
