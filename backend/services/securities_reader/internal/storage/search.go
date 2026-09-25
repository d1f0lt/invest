package storage

import (
	"context"
	"fmt"
	"strings"
)













func (s *Store) SearchSecurities(ctx context.Context, query string, limit int) ([]PriceView, error) {
	like := escapeLike(query)
	rows, err := s.db.QueryContext(ctx, `
		WITH m AS (
			SELECT
				lp.secid, lp.board, sec.short_name, sec.sec_name, sec.isin, sec.currency,
				lp.last_price, lp.open_price, lp.high_price, lp.low_price,
				lp.value_today, lp.volume_today, lp.trading_status, lp.moex_update_time, lp.collected_at,
				lp.prev_close,
				lower(lp.secid) AS t,
				lower(coalesce(sec.isin, '')) AS i,
				replace(lower(coalesce(sec.short_name, '')), 'ё', 'е') AS sn,
				replace(lower(coalesce(sec.sec_name, '')), 'ё', 'е') AS fn
			FROM latest_prices lp
			LEFT JOIN securities sec ON sec.secid = lp.secid AND sec.board = lp.board
		), r AS (
			SELECT m.*,
				CASE
					WHEN t = $1::text OR i = $1::text THEN 0
					WHEN t LIKE $2::text || '%' THEN 1
					WHEN sn LIKE $2::text || '%' OR fn LIKE $2::text || '%' THEN 2
					WHEN sn LIKE '% ' || $2::text || '%' OR fn LIKE '% ' || $2::text || '%'
						OR sn LIKE '%"' || $2::text || '%' OR fn LIKE '%"' || $2::text || '%'
						OR sn LIKE '%«' || $2::text || '%' OR fn LIKE '%«' || $2::text || '%' THEN 3
					WHEN t LIKE '%' || $2::text || '%' OR sn LIKE '%' || $2::text || '%' OR fn LIKE '%' || $2::text || '%'
						OR (length($1::text) >= 4 AND i LIKE $2::text || '%') THEN 4
				END AS rank
			FROM m
		)
		SELECT
			top.secid, top.board, top.short_name, top.sec_name, top.isin, top.currency,
			top.last_price, top.open_price, top.high_price, top.low_price,
			top.value_today, top.volume_today, top.trading_status, top.moex_update_time, top.collected_at,
			top.prev_close
		FROM (
			SELECT * FROM r
			WHERE rank IS NOT NULL
			ORDER BY rank,
				CASE board WHEN 'TQBR' THEN 0 WHEN 'TQTF' THEN 1 WHEN 'TQOB' THEN 2 WHEN 'TQCB' THEN 3 ELSE 4 END,
				secid, board
			LIMIT $3::int
		) top
		ORDER BY top.rank,
			CASE top.board WHEN 'TQBR' THEN 0 WHEN 'TQTF' THEN 1 WHEN 'TQOB' THEN 2 WHEN 'TQCB' THEN 3 ELSE 4 END,
			top.secid, top.board
	`, query, like, limit)
	if err != nil {
		return nil, fmt.Errorf("search securities: %w", err)
	}
	defer rows.Close()
	return scanPriceRows(rows)
}



func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
