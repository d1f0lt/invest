CREATE TABLE IF NOT EXISTS security_metadata (
    secid TEXT PRIMARY KEY,
    description JSONB,
    issuer TEXT,
    info_updated_at TIMESTAMPTZ,
    dividends JSONB,
    dividends_updated_at TIMESTAMPTZ
);
