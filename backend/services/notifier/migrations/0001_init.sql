-- notifier: price alerts + Telegram delivery.
-- Idempotent. user_id has no FK to users (same as portfolio: services
-- share the database but not user ownership); (secid, board) references
-- price_updater's securities, so price_updater's migrations must run first
-- (mount order 10_price_updater < 40_notifier in docker-compose.yml).

CREATE TABLE IF NOT EXISTS alerts (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              UUID NOT NULL,
    secid                TEXT NOT NULL,
    board                TEXT NOT NULL,
    -- 'above': fires when last_price >= target_price; 'below': <=.
    -- Derived from target vs base.
    direction            TEXT NOT NULL CHECK (direction IN ('above', 'below')),
    -- Quote when the alert was created or last updated, in quote units
    -- (bonds: % of face value).
    base_price           NUMERIC NOT NULL CHECK (base_price > 0),
    target_price         NUMERIC NOT NULL CHECK (target_price > 0),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- base/target last set (creation or UpdateAlert).
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Set when the notification was sent; the alert is then no longer
    -- checked until the user updates it (UpdateAlert clears both).
    triggered_at         TIMESTAMPTZ,
    triggered_price      NUMERIC,
    FOREIGN KEY (secid, board) REFERENCES securities (secid, board)
);

CREATE INDEX IF NOT EXISTS idx_alerts_user_id ON alerts (user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_alerts_active ON alerts (secid, board) WHERE triggered_at IS NULL;

-- One Telegram chat per user and one user per chat.
CREATE TABLE IF NOT EXISTS telegram_links (
    user_id    UUID PRIMARY KEY,
    chat_id    BIGINT NOT NULL UNIQUE,
    username   TEXT,
    linked_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One-time tokens for the t.me/<bot>?start=<token> deep link. Only the
-- SHA-256 of the token is stored (same idea as users.refresh_tokens).
CREATE TABLE IF NOT EXISTS telegram_link_tokens (
    token_hash  TEXT PRIMARY KEY,
    user_id     UUID NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_telegram_link_tokens_user_id ON telegram_link_tokens (user_id);
