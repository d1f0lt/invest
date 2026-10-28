package grpcserver

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lib/pq"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"invest/backend/services/feed/internal/authmd"
	feedpb "invest/backend/services/feed/proto"
)

const (
	feedPageSize   = 30
	feedDateFormat = "2006-01-02"
)

var feedTimezone = time.FixedZone("MSK", 3*60*60)

type Server struct {
	feedpb.UnimplementedFeedServiceServer
	DB  *sql.DB
	Log *slog.Logger
}

func validateDraft(d *feedpb.CreatePostRequest, earliestDate time.Time) string {
	d.Title = strings.TrimSpace(d.Title)
	d.Body = strings.TrimSpace(d.Body)
	if d.Title == "" || utf8.RuneCountInString(d.Title) > 120 {
		return "Введите заголовок до 120 символов"
	}
	if d.Body == "" || utf8.RuneCountInString(d.Body) > 8000 {
		return "Введите описание до 8000 символов"
	}
	if len(d.Tickers) < 1 || len(d.Tickers) > 20 {
		return "Выберите от 1 до 20 бумаг"
	}
	seen := make(map[string]bool, len(d.Tickers))
	for i, ticker := range d.Tickers {
		ticker = strings.ToUpper(strings.TrimSpace(ticker))
		if ticker == "" || len(ticker) > 32 || seen[ticker] {
			return "Выберите разные бумаги из справочника"
		}
		seen[ticker] = true
		d.Tickers[i] = ticker
	}
	switch d.Kind {
	case "news":
		if d.Board != "" || d.ChangePercent != nil || d.Recommendation != "" || d.TargetDate != "" {
			return "Поля прогноза недоступны для новости"
		}
	case "forecast":
		if len(d.Tickers) != 1 || d.Board == "" || len(d.Board) > 16 {
			return "Выберите одну бумагу из справочника"
		}
		change := d.ChangePercent
		if change == nil || math.IsNaN(*change) || *change == 0 || *change < -100 || *change > 10000 {
			return "Укажите ненулевое изменение цены от −100% до +10 000%"
		}
		switch d.Recommendation {
		case "hold":
		case "buy":
			if *change < 0 {
				return "При прогнозе снижения нельзя рекомендовать покупку"
			}
		case "sell":
			if *change > 0 {
				return "При прогнозе роста нельзя рекомендовать продажу"
			}
		default:
			return "Выберите рекомендацию"
		}
		if _, err := time.Parse(feedDateFormat, d.TargetDate); err != nil ||
			d.TargetDate < earliestDate.In(feedTimezone).Format(feedDateFormat) {
			return "Выберите будущую дату прогноза"
		}
	default:
		return "Выберите тип публикации"
	}
	return ""
}

func (s *Server) CreatePost(ctx context.Context, req *feedpb.CreatePostRequest) (*feedpb.CreatePostResponse, error) {
	author, err := authmd.UserID(ctx)
	if err != nil {
		return nil, err
	}
	if message := validateDraft(req, time.Now().In(feedTimezone).AddDate(0, 0, 1)); message != "" {
		return nil, status.Error(codes.FailedPrecondition, message)
	}

	var id int64
	err = s.DB.QueryRowContext(ctx, `
		INSERT INTO feed_posts (author_id, kind, title, body, tickers, board,
			change_percent, recommendation, target_date, baseline_price)
		SELECT $1, $2, $3, $4, $5, nullif($6, ''), $7, nullif($8, ''),
			nullif($9, '')::date, price.last_price
		FROM users u
		LEFT JOIN latest_prices price ON price.secid = ($5::text[])[1]
			AND price.board = nullif($6, '') AND price.last_price > 0
		WHERE u.id = $1 AND u.is_active
			AND NOT EXISTS (
				SELECT 1 FROM unnest($5::text[]) AS ticker(secid)
				WHERE NOT EXISTS (SELECT 1 FROM securities s WHERE s.secid = ticker.secid)
			)
			AND ($2 = 'news' OR (price.last_price IS NOT NULL
				AND nullif($9, '')::date > (now() AT TIME ZONE 'Europe/Moscow')::date
				AND EXISTS (SELECT 1 FROM securities s WHERE s.secid = price.secid AND s.board = price.board)))
		RETURNING id`, author, req.Kind, req.Title, req.Body, pq.Array(req.Tickers),
		req.Board, req.ChangePercent, req.Recommendation, req.TargetDate).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Error(codes.FailedPrecondition, "Проверьте бумаги и дату. Для прогноза нужна доступная котировка")
	}
	if err != nil {
		return nil, s.databaseError(err)
	}
	return &feedpb.CreatePostResponse{Id: id}, nil
}

func (s *Server) ListPosts(ctx context.Context, req *feedpb.ListPostsRequest) (*feedpb.ListPostsResponse, error) {
	if _, err := authmd.UserID(ctx); err != nil {
		return nil, err
	}
	kind, newsStatus := req.GetKind(), req.GetNewsStatus()

	if kind == "" {
		kind = "news"
	}
	if (kind != "news" && kind != "forecast" && kind != "all") ||
		(newsStatus != "" && (kind != "news" || (newsStatus != "verified" && newsStatus != "unverified"))) {
		return nil, status.Error(codes.InvalidArgument, "Некорректный фильтр публикаций")
	}
	tickers := req.GetTickers()
	if len(tickers) > 20 || (kind == "forecast" && len(tickers) > 1) {
		return nil, status.Error(codes.InvalidArgument, "Слишком много бумаг в фильтре")
	}
	var minRating *int
	if raw := req.GetMinRating(); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > 100 || (kind == "news" && newsStatus != "unverified") {
			return nil, status.Error(codes.InvalidArgument, "Некорректный фильтр рейтинга")
		}
		minRating = &value
	}
	newsRating, newsOK := parseFeedRating(req.GetMinNewsRating())
	forecastRating, forecastOK := parseFeedRating(req.GetMinForecastRating())
	if !newsOK || !forecastOK || (kind != "all" && (newsRating != nil || forecastRating != nil)) ||
		(kind == "all" && minRating != nil) {
		return nil, status.Error(codes.InvalidArgument, "Некорректный фильтр рейтинга")
	}
	var before *int64
	if raw := req.GetBefore(); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			return nil, status.Error(codes.InvalidArgument, "Некорректный курсор ленты")
		}
		before = &value
	}
	var beforeAt *time.Time
	if raw := req.GetBeforeAt(); raw != "" {
		value, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil || before == nil {
			return nil, status.Error(codes.InvalidArgument, "Некорректный курсор ленты")
		}
		beforeAt = &value
	}

	rows, err := s.DB.QueryContext(ctx, `
		SELECT p.id, p.kind, p.title, p.body, p.tickers, coalesce(u.username, 'Пользователь'), u.is_admin,
			p.created_at, p.verified_at IS NOT NULL, score.rating, coalesce(score.sample_size, 0),
			coalesce(p.board, ''), p.change_percent, coalesce(p.recommendation, ''),
			coalesce(to_char(p.target_date, 'YYYY-MM-DD'), '')
		FROM feed_posts p
		JOIN users u ON u.id = p.author_id AND u.is_active
		LEFT JOIN feed_author_scores score ON score.author_id = p.author_id AND score.kind = p.kind
		WHERE ($1 = 'all' OR p.kind = $1) AND p.deleted_at IS NULL
			AND (p.kind = 'news' OR p.target_date >= (now() AT TIME ZONE 'Europe/Moscow')::date)
			AND ($2 = '' OR ($2 = 'verified') = (p.verified_at IS NOT NULL))
			AND (coalesce(cardinality($3::text[]), 0) = 0 OR p.tickers && $3)
			AND ($4::int IS NULL OR score.rating >= $4)
			AND (p.kind <> 'news' OR $5::int IS NULL OR score.rating >= $5)
			AND (p.kind <> 'forecast' OR $6::int IS NULL OR score.rating >= $6)
			AND ($7::bigint IS NULL OR (p.created_at, p.id) <
				(coalesce($8::timestamptz, (SELECT created_at FROM feed_posts WHERE id = $7)), $7))
		ORDER BY p.created_at DESC, p.id DESC LIMIT $9`,
		kind, newsStatus, pq.Array(tickers), minRating, newsRating, forecastRating, before, beforeAt, feedPageSize+1)
	if err != nil {
		return nil, s.databaseError(err)
	}
	defer rows.Close()
	posts := make([]*feedpb.Post, 0, feedPageSize+1)
	for rows.Next() {
		post, err := scanPost(rows)
		if err != nil {
			return nil, s.databaseError(err)
		}
		posts = append(posts, post)
	}
	if err := rows.Err(); err != nil {
		return nil, s.databaseError(err)
	}
	result := &feedpb.ListPostsResponse{Posts: posts}
	if len(posts) > feedPageSize {
		result.Posts = posts[:feedPageSize]
		last := result.Posts[len(result.Posts)-1]
		result.Next = &last.Id
		result.NextAt = last.CreatedAt
	}
	return result, nil
}

func scanPost(row interface{ Scan(...any) error }) (*feedpb.Post, error) {
	var post feedpb.Post
	var createdAt time.Time
	if err := row.Scan(&post.Id, &post.Kind, &post.Title, &post.Body, pq.Array(&post.Tickers),
		&post.Author, &post.AuthorIsAdmin, &createdAt, &post.Verified, &post.Rating, &post.RatingSamples,
		&post.Board, &post.ChangePercent, &post.Recommendation, &post.TargetDate); err != nil {
		return nil, err
	}
	post.CreatedAt = timestamppb.New(createdAt)
	return &post, nil
}

func parseFeedRating(raw string) (*int, bool) {
	if raw == "" {
		return nil, true
	}
	value, err := strconv.Atoi(raw)
	return &value, err == nil && value >= 0 && value <= 100
}

func (s *Server) databaseError(err error) error {
	s.Log.Error("feed database operation failed", "error", err)
	return status.Error(codes.Internal, "Не удалось выполнить операцию с лентой")
}
