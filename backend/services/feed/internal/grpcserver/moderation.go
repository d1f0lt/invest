package grpcserver

import (
	"context"
	"database/sql"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"invest/backend/services/feed/internal/authmd"
	feedpb "invest/backend/services/feed/proto"
)

func (s *Server) ModeratePost(ctx context.Context, req *feedpb.ModeratePostRequest) (*feedpb.ModeratePostResponse, error) {
	user, err := authmd.UserID(ctx)
	if err != nil {
		return nil, err
	}
	postID := req.GetPostId()
	if postID <= 0 {
		return nil, status.Error(codes.InvalidArgument, "Некорректный номер публикации")
	}

	var result sql.Result
	switch req.GetAction() {
	case "verify_news":
		result, err = s.DB.ExecContext(ctx, `
			UPDATE feed_posts p SET verified_at = coalesce(p.verified_at, now())
			FROM users actor
			WHERE p.id = $1 AND p.kind = 'news' AND p.deleted_at IS NULL
			  AND actor.id = $2 AND actor.is_active AND actor.is_admin
		`, postID, user)
	case "delete_post":
		result, err = s.DB.ExecContext(ctx, `
			UPDATE feed_posts p SET deleted_at = now(),
			    rejected_at = CASE WHEN p.kind = 'news' AND p.verified_at IS NULL
			        AND actor.is_admin AND actor.id <> p.author_id THEN now() END
			FROM users actor
			WHERE p.id = $1 AND p.deleted_at IS NULL AND actor.id = $2 AND actor.is_active
			  AND (actor.is_admin OR actor.id = p.author_id)
		`, postID, user)
	case "delete_comment":
		if req.GetCommentId() <= 0 {
			return nil, status.Error(codes.InvalidArgument, "Некорректный номер комментария")
		}
		result, err = s.DB.ExecContext(ctx, `
			DELETE FROM feed_comments c USING feed_posts p, users actor
			WHERE c.id = $2 AND c.post_id = $1 AND p.id = c.post_id AND p.deleted_at IS NULL
			  AND actor.id = $3 AND actor.is_active
			  AND (actor.is_admin OR actor.id = p.author_id OR actor.id = c.author_id)
		`, postID, req.GetCommentId(), user)
	default:
		return nil, status.Error(codes.InvalidArgument, "Неизвестное действие модерации")
	}
	if err != nil {
		return nil, s.databaseError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, s.databaseError(err)
	}
	if count == 0 {
		return nil, status.Error(codes.PermissionDenied, "Запись недоступна или недостаточно прав")
	}
	return &feedpb.ModeratePostResponse{}, nil
}
