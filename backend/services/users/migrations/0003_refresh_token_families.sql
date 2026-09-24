-- Refresh token rotation families: every token issued by Login starts a
-- new family (DEFAULT), every rotated token inherits its parent's
-- family_id. Presenting an already-revoked token (replay/theft) revokes
-- the WHOLE family - see grpcserver.RefreshToken.
--
-- On an existing database each pre-migration row becomes its own family
-- (gen_random_uuid() is volatile and evaluated per row), which is exactly
-- right: those tokens predate rotation tracking.
ALTER TABLE refresh_tokens
    ADD COLUMN IF NOT EXISTS family_id UUID NOT NULL DEFAULT gen_random_uuid();

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family_id
    ON refresh_tokens (family_id);

-- Supports the periodic cleanup DELETE (cmd/main.go -> internal/cleanup):
-- rows are removed once expires_at is in the past.
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at
    ON refresh_tokens (expires_at);
