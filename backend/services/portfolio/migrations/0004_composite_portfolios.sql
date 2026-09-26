-- Composite portfolios: a portfolio built from other portfolios of the same
-- user. It has no trades or cash operations of its own - the service reads
-- its members' ledgers together. A portfolio is composite iff it has rows
-- here; members are always ordinary portfolios (checked by the service).
CREATE TABLE IF NOT EXISTS portfolio_members (
    portfolio_id UUID NOT NULL REFERENCES portfolios (id) ON DELETE CASCADE,
    member_id    UUID NOT NULL REFERENCES portfolios (id) ON DELETE CASCADE,
    position     INT  NOT NULL,
    PRIMARY KEY (portfolio_id, member_id),
    CHECK (portfolio_id <> member_id)
);

CREATE INDEX IF NOT EXISTS idx_portfolio_members_member_id ON portfolio_members (member_id);
