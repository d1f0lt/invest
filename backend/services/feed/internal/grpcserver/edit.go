package grpcserver

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"invest/backend/services/feed/internal/authmd"
	feedpb "invest/backend/services/feed/proto"
)

func (s *Server) UpdatePost(ctx context.Context, req *feedpb.UpdatePostRequest) (*feedpb.Post, error) {
	user, err := authmd.UserID(ctx)
	if err != nil {
		return nil, err
	}
	draft := req.GetDraft()
	if req.GetPostId() <= 0 || draft == nil {
		return nil, status.Error(codes.InvalidArgument, "Укажите публикацию и новые данные")
	}
	if message := validateDraft(draft, time.Now()); message != "" {
		return nil, status.Error(codes.FailedPrecondition, message)
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, s.databaseError(err)
	}
	defer tx.Rollback()

	var kind string
	err = tx.QueryRowContext(ctx, `
		SELECT p.kind
		FROM feed_posts p
		JOIN users actor ON actor.id = $2 AND actor.is_active
		WHERE p.id = $1 AND p.deleted_at IS NULL
		  AND (actor.is_admin OR actor.id = p.author_id)
		FOR UPDATE OF p
	`, req.GetPostId(), user).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Error(codes.PermissionDenied, "Публикация недоступна или недостаточно прав")
	}
	if err != nil {
		return nil, s.databaseError(err)
	}
	if kind != draft.Kind {
		return nil, status.Error(codes.FailedPrecondition, "Тип опубликованной записи менять нельзя")
	}

	err = tx.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT p.id, (p.kind = 'forecast' AND (
				p.tickers IS DISTINCT FROM $4::text[] OR p.board IS DISTINCT FROM nullif($5, '')
				OR p.change_percent IS DISTINCT FROM $6::numeric OR p.target_date IS DISTINCT FROM nullif($8, '')::date
			)) AS revised
			FROM feed_posts p WHERE p.id = $1
		)
		UPDATE feed_posts p SET title = $2, body = $3, tickers = $4,
			board = nullif($5, ''), change_percent = $6, recommendation = nullif($7, ''),
			target_date = nullif($8, '')::date,
			verified_at = CASE WHEN p.kind = 'news' AND (p.title <> $2 OR p.body <> $3 OR p.tickers <> $4)
				THEN NULL ELSE p.verified_at END,
			baseline_price = CASE WHEN candidate.revised THEN price.last_price ELSE p.baseline_price END,
			forecast_revised_at = CASE WHEN candidate.revised THEN now() ELSE p.forecast_revised_at END
		FROM candidate
		LEFT JOIN latest_prices price ON price.secid = ($4::text[])[1]
		  AND price.board = nullif($5, '') AND price.last_price > 0
		WHERE p.id = candidate.id AND (
			p.kind = 'news' OR (
				p.target_date >= (now() AT TIME ZONE 'Europe/Moscow')::date
				AND (NOT candidate.revised OR (
					nullif($8, '')::date > (now() AT TIME ZONE 'Europe/Moscow')::date
					AND price.last_price IS NOT NULL
					AND EXISTS (SELECT 1 FROM securities s WHERE s.secid = price.secid AND s.board = price.board)
				))
			)
		)
		AND NOT EXISTS (
			SELECT 1 FROM unnest($4::text[]) AS ticker(secid)
			WHERE NOT EXISTS (SELECT 1 FROM securities s WHERE s.secid = ticker.secid)
		)
		RETURNING p.id
	`, req.GetPostId(), draft.Title, draft.Body, pq.Array(draft.Tickers), draft.Board,
		draft.ChangePercent, draft.Recommendation, draft.TargetDate).Scan(new(int64))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Error(codes.FailedPrecondition, "Проверьте бумаги, дату и наличие котировки. Завершённый прогноз изменить нельзя")
	}
	if err != nil {
		return nil, s.databaseError(err)
	}

	post, err := scanPost(tx.QueryRowContext(ctx, `
		SELECT p.id, p.kind, p.title, p.body, p.tickers, coalesce(u.username, 'Пользователь'), u.is_admin,
			p.created_at, p.verified_at IS NOT NULL, score.rating, coalesce(score.sample_size, 0),
			coalesce(p.board, ''), p.change_percent, coalesce(p.recommendation, ''),
			coalesce(to_char(p.target_date, 'YYYY-MM-DD'), '')
		FROM feed_posts p
		JOIN users u ON u.id = p.author_id
		LEFT JOIN feed_author_scores score ON score.author_id = p.author_id AND score.kind = p.kind
		WHERE p.id = $1
	`, req.GetPostId()))
	if err != nil {
		return nil, s.databaseError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, s.databaseError(err)
	}
	return post, nil
}
