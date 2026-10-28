ALTER TABLE feed_posts ADD COLUMN IF NOT EXISTS forecast_revised_at TIMESTAMPTZ;

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
          AND start_at >= ((((coalesce(p.forecast_revised_at, p.created_at) AT TIME ZONE 'Europe/Moscow')::date + 1))::timestamp AT TIME ZONE 'Europe/Moscow')
        ORDER BY start_at DESC LIMIT 1
    ) c ON true
    WHERE p.kind = 'forecast' AND p.deleted_at IS NULL
      AND p.target_date < (now() AT TIME ZONE 'Europe/Moscow')::date
) scored
GROUP BY author_id, kind;
