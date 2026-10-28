CREATE TABLE IF NOT EXISTS feed_posts (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    author_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('news', 'forecast')),
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    tickers TEXT[] NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    verified_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    rejected_at TIMESTAMPTZ,
    board TEXT,
    change_percent NUMERIC,
    recommendation TEXT,
    target_date DATE,
    baseline_price NUMERIC,
    CHECK (rejected_at IS NULL OR (kind = 'news' AND deleted_at IS NOT NULL AND verified_at IS NULL)),
    CHECK (
        (kind = 'news' AND cardinality(tickers) BETWEEN 1 AND 20
            AND board IS NULL AND change_percent IS NULL AND recommendation IS NULL
            AND target_date IS NULL AND baseline_price IS NULL)
        OR
        (kind = 'forecast' AND cardinality(tickers) = 1 AND verified_at IS NULL
            AND board IS NOT NULL AND change_percent IS NOT NULL
            AND change_percent BETWEEN -100 AND 10000 AND change_percent <> 0
            AND recommendation IS NOT NULL AND recommendation IN ('buy', 'sell', 'hold')
            AND (recommendation = 'hold' OR (recommendation = 'buy' AND change_percent > 0)
                OR (recommendation = 'sell' AND change_percent < 0))
            AND target_date IS NOT NULL AND baseline_price IS NOT NULL AND baseline_price > 0)
    )
);

CREATE INDEX IF NOT EXISTS feed_posts_kind_id ON feed_posts (kind, id DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS feed_posts_tickers ON feed_posts USING GIN (tickers)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS feed_posts_author_kind ON feed_posts (author_id, kind, id DESC);

CREATE OR REPLACE VIEW feed_author_scores AS
SELECT author_id, kind, count(*) AS sample_size,
       round(100.0 * count(*) FILTER (WHERE correct) / count(*))::int AS rating
FROM (
    SELECT author_id, kind, verified_at IS NOT NULL AS correct
    FROM feed_posts
    WHERE kind = 'news' AND ((verified_at IS NOT NULL AND deleted_at IS NULL) OR rejected_at IS NOT NULL)
    UNION ALL
    SELECT p.author_id, p.kind,
        CASE WHEN p.change_percent > 0 THEN c.close >= p.baseline_price * (1 + p.change_percent / 100)
             ELSE c.close <= p.baseline_price * (1 + p.change_percent / 100) END
    FROM feed_posts p
    JOIN candle_history_sync s ON s.board = p.board AND s.synced_through >= p.target_date
    JOIN LATERAL (
        SELECT close FROM candles
        WHERE secid = p.tickers[1] AND board = p.board AND interval = '1d'
          AND start_at < ((p.target_date + 1)::timestamp AT TIME ZONE 'Europe/Moscow')
          AND start_at >= ((((p.created_at AT TIME ZONE 'Europe/Moscow')::date + 1))::timestamp AT TIME ZONE 'Europe/Moscow')
        ORDER BY start_at DESC LIMIT 1
    ) c ON true
    WHERE p.kind = 'forecast' AND p.deleted_at IS NULL
      AND p.target_date < (now() AT TIME ZONE 'Europe/Moscow')::date
) scored
GROUP BY author_id, kind;
