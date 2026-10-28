package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"invest/backend/services/gateway/internal/auth"
	feedpb "invest/backend/services/gateway/internal/feedpb"
)

type commentView struct {
	ID            int64     `json:"id"`
	Author        string    `json:"author"`
	AuthorIsAdmin *bool     `json:"author_is_admin,omitempty"`
	CanDelete     bool      `json:"can_delete"`
	Body          string    `json:"body"`
	CreatedAt     time.Time `json:"created_at"`
}

func viewComment(c *feedpb.Comment) commentView {
	return commentView{ID: c.GetId(), Author: c.GetAuthor(), AuthorIsAdmin: c.AuthorIsAdmin, CanDelete: c.GetCanDelete(), Body: c.GetBody(), CreatedAt: c.GetCreatedAt().AsTime()}
}

func viewReactions(r *feedpb.Reactions) map[string]int64 {
	return map[string]int64{"likes": r.GetLikes(), "dislikes": r.GetDislikes(), "mine": int64(r.GetMine())}
}

func discussionPostID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "Некорректный номер публикации")
		return 0, false
	}
	return id, true
}

func (h *Handlers) handleGetDiscussion(w http.ResponseWriter, r *http.Request) {
	id, ok := discussionPostID(w, r)
	if !ok {
		return
	}
	var before int64
	if raw := r.URL.Query().Get("before"); raw != "" {
		var err error
		before, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || before <= 0 {
			writeError(w, http.StatusBadRequest, "Некорректный курсор комментариев")
			return
		}
	}

	ctx, cancel := h.callCtx(r)
	defer cancel()
	userID, _ := userIDFromContext(ctx)
	resp, err := h.Upstream.Feed.GetDiscussion(auth.WithUserID(ctx, userID), &feedpb.GetDiscussionRequest{PostId: id, Before: before})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	comments := make([]commentView, 0, len(resp.GetComments()))
	for _, comment := range resp.GetComments() {
		comments = append(comments, viewComment(comment))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"comments": comments, "reactions": viewReactions(resp.GetReactions()), "next": resp.Next,
		"can_moderate": resp.GetCanModerate(), "can_delete_post": resp.GetCanDeletePost(), "verified": resp.GetVerified(),
	})
}

func (h *Handlers) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	id, ok := discussionPostID(w, r)
	if !ok {
		return
	}
	var payload struct {
		Body string `json:"body"`
	}
	if !decodeFeedJSON(w, r, 24<<10, &payload) {
		return
	}

	ctx, cancel := h.callCtx(r)
	defer cancel()
	userID, _ := userIDFromContext(ctx)
	resp, err := h.Upstream.Feed.CreateComment(auth.WithUserID(ctx, userID), &feedpb.CreateCommentRequest{PostId: id, Body: payload.Body})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, viewComment(resp))
}

func (h *Handlers) handleSetReaction(w http.ResponseWriter, r *http.Request) {
	id, ok := discussionPostID(w, r)
	if !ok {
		return
	}
	var payload struct {
		Value *int32 `json:"value"`
	}
	if !decodeFeedJSON(w, r, 1024, &payload) {
		return
	}
	if payload.Value == nil {
		writeError(w, http.StatusBadRequest, "Укажите реакцию")
		return
	}

	ctx, cancel := h.callCtx(r)
	defer cancel()
	userID, _ := userIDFromContext(ctx)
	resp, err := h.Upstream.Feed.SetReaction(auth.WithUserID(ctx, userID), &feedpb.SetReactionRequest{PostId: id, Value: *payload.Value})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, viewReactions(resp))
}

func (h *Handlers) handleModeratePost(w http.ResponseWriter, r *http.Request) {
	id, ok := discussionPostID(w, r)
	if !ok {
		return
	}
	var payload struct {
		Action    string `json:"action"`
		CommentID int64  `json:"comment_id"`
	}
	if !decodeFeedJSON(w, r, 1024, &payload) {
		return
	}
	ctx, cancel := h.callCtx(r)
	defer cancel()
	user, _ := userIDFromContext(ctx)
	_, err := h.Upstream.Feed.ModeratePost(auth.WithUserID(ctx, user), &feedpb.ModeratePostRequest{
		PostId: id, Action: payload.Action, CommentId: payload.CommentID,
	})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
