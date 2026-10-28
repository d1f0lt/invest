package grpcserver

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"invest/backend/services/feed/internal/authmd"
	feedpb "invest/backend/services/feed/proto"
)

func (s *Server) GetDiscussion(ctx context.Context, req *feedpb.GetDiscussionRequest) (*feedpb.GetDiscussionResponse, error) {
	user, err := authmd.UserID(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetPostId() <= 0 || req.GetBefore() < 0 {
		return nil, status.Error(codes.InvalidArgument, "Некорректный запрос комментариев")
	}
	reactions, err := s.getReactions(ctx, req.GetPostId(), user)
	if err != nil {
		return nil, err
	}
	result := &feedpb.GetDiscussionResponse{Reactions: reactions}
	if err := s.DB.QueryRowContext(ctx, `
		SELECT p.verified_at IS NOT NULL,
		    actor.is_admin, actor.is_admin OR p.author_id = actor.id
		FROM feed_posts p
		JOIN users actor ON actor.id = $2 AND actor.is_active
		WHERE p.id = $1 AND p.deleted_at IS NULL
	`, req.GetPostId(), user).Scan(&result.Verified, &result.CanModerate, &result.CanDeletePost); err != nil {
		return nil, s.discussionError(err)
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT c.id, coalesce(u.username, 'Пользователь'), u.is_admin, c.body, c.created_at,
            $4::boolean OR c.author_id = $5
		FROM feed_comments c
		JOIN users u ON u.id = c.author_id AND u.is_active
		JOIN feed_posts p ON p.id = c.post_id AND p.deleted_at IS NULL
		WHERE c.post_id = $1 AND ($2::bigint = 0 OR c.id < $2)
		ORDER BY c.id DESC LIMIT $3
	`, req.GetPostId(), req.GetBefore(), feedPageSize+1, result.CanDeletePost, user)
	if err != nil {
		return nil, s.databaseError(err)
	}
	defer rows.Close()

	for rows.Next() {
		var comment feedpb.Comment
		var createdAt time.Time
		if err := rows.Scan(&comment.Id, &comment.Author, &comment.AuthorIsAdmin, &comment.Body, &createdAt, &comment.CanDelete); err != nil {
			return nil, s.databaseError(err)
		}
		comment.CreatedAt = timestamppb.New(createdAt)
		result.Comments = append(result.Comments, &comment)
	}
	if err := rows.Err(); err != nil {
		return nil, s.databaseError(err)
	}
	if len(result.Comments) > feedPageSize {
		result.Comments = result.Comments[:feedPageSize]
		result.Next = &result.Comments[feedPageSize-1].Id
	}
	return result, nil
}

func (s *Server) CreateComment(ctx context.Context, req *feedpb.CreateCommentRequest) (*feedpb.Comment, error) {
	user, err := authmd.UserID(ctx)
	if err != nil {
		return nil, err
	}
	body := strings.TrimSpace(req.GetBody())
	if req.GetPostId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "Некорректный номер публикации")
	}
	if body == "" || utf8.RuneCountInString(body) > 2000 {
		return nil, status.Error(codes.FailedPrecondition, "Введите комментарий до 2000 символов")
	}
	comment := &feedpb.Comment{Body: body, CanDelete: true}
	var createdAt time.Time
	err = s.DB.QueryRowContext(ctx, `
		WITH author AS (
			SELECT id, coalesce(username, 'Пользователь') AS name, is_admin
			FROM users WHERE id = $2 AND is_active
		), inserted AS (
			INSERT INTO feed_comments(post_id, author_id, body)
			SELECT p.id, author.id, $3 FROM feed_posts p
			JOIN users owner ON owner.id = p.author_id AND owner.is_active
			CROSS JOIN author
			WHERE p.id = $1 AND p.deleted_at IS NULL
			RETURNING id, created_at
		)
		SELECT inserted.id, author.name, author.is_admin, inserted.created_at FROM inserted CROSS JOIN author
	`, req.GetPostId(), user, body).Scan(&comment.Id, &comment.Author, &comment.AuthorIsAdmin, &createdAt)
	if err != nil {
		return nil, s.discussionError(err)
	}
	comment.CreatedAt = timestamppb.New(createdAt)
	return comment, nil
}

func (s *Server) SetReaction(ctx context.Context, req *feedpb.SetReactionRequest) (*feedpb.Reactions, error) {
	user, err := authmd.UserID(ctx)
	if err != nil {
		return nil, err
	}
	postID, value := req.GetPostId(), req.GetValue()
	if postID <= 0 || value < -1 || value > 1 {
		return nil, status.Error(codes.InvalidArgument, "Некорректная реакция")
	}
	if value == 0 {
		_, err = s.DB.ExecContext(ctx, `DELETE FROM feed_reactions WHERE post_id = $1 AND user_id = $2`, postID, user)
	} else {
		_, err = s.DB.ExecContext(ctx, `
			INSERT INTO feed_reactions(post_id, user_id, value)
			SELECT p.id, u.id, $3 FROM feed_posts p
			JOIN users owner ON owner.id = p.author_id AND owner.is_active
			JOIN users u ON u.id = $2 AND u.is_active
			WHERE p.id = $1 AND p.deleted_at IS NULL
			ON CONFLICT (post_id, user_id) DO UPDATE SET value = excluded.value
		`, postID, user, value)
	}
	if err != nil {
		return nil, s.databaseError(err)
	}
	return s.getReactions(ctx, postID, user)
}

func (s *Server) getReactions(ctx context.Context, postID int64, user string) (*feedpb.Reactions, error) {
	var reactions feedpb.Reactions
	err := s.DB.QueryRowContext(ctx, `
		SELECT count(*) FILTER (WHERE r.value = 1), count(*) FILTER (WHERE r.value = -1),
			coalesce(max(r.value) FILTER (WHERE r.user_id = $2), 0)
		FROM feed_posts p
		JOIN users u ON u.id = p.author_id AND u.is_active
		LEFT JOIN feed_reactions r ON r.post_id = p.id
		WHERE p.id = $1 AND p.deleted_at IS NULL
		GROUP BY p.id
	`, postID, user).Scan(&reactions.Likes, &reactions.Dislikes, &reactions.Mine)
	if err != nil {
		return nil, s.discussionError(err)
	}
	return &reactions, nil
}

func (s *Server) discussionError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return status.Error(codes.NotFound, "Публикация недоступна")
	}
	return s.databaseError(err)
}
