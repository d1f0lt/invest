package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"invest/backend/services/gateway/internal/auth"
	feedpb "invest/backend/services/gateway/internal/feedpb"
)

type feedPostBody struct {
	Kind           string   `json:"kind"`
	Title          string   `json:"title"`
	Body           string   `json:"body"`
	Tickers        []string `json:"tickers"`
	Board          string   `json:"board,omitempty"`
	ChangePercent  *float64 `json:"change_percent,omitempty"`
	Recommendation string   `json:"recommendation,omitempty"`
	TargetDate     string   `json:"target_date,omitempty"`
}

type feedPostView struct {
	feedPostBody
	ID            int64     `json:"id"`
	Author        string    `json:"author"`
	AuthorIsAdmin *bool     `json:"author_is_admin,omitempty"`
	Rating        *int32    `json:"rating"`
	RatingSamples int32     `json:"rating_samples"`
	Verified      bool      `json:"verified"`
	CreatedAt     time.Time `json:"created_at"`
}

func decodeFeedJSON(w http.ResponseWriter, r *http.Request, limit int64, dst any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	decoder.DisallowUnknownFields()
	if decoder.Decode(dst) != nil || decoder.Decode(new(any)) != io.EOF {
		writeError(w, http.StatusBadRequest, "Некорректные данные")
		return false
	}
	return true
}

func (draft feedPostBody) toRequest() *feedpb.CreatePostRequest {
	return &feedpb.CreatePostRequest{
		Kind: draft.Kind, Title: draft.Title, Body: draft.Body, Tickers: draft.Tickers,
		Board: draft.Board, ChangePercent: draft.ChangePercent,
		Recommendation: draft.Recommendation, TargetDate: draft.TargetDate,
	}
}

func (h *Handlers) handleUpdateFeedPost(w http.ResponseWriter, r *http.Request) {
	id, ok := discussionPostID(w, r)
	if !ok {
		return
	}
	var draft feedPostBody
	if !decodeFeedJSON(w, r, 96<<10, &draft) {
		return
	}
	ctx, cancel := h.callCtx(r)
	defer cancel()
	userID, _ := userIDFromContext(ctx)
	post, err := h.Upstream.Feed.UpdatePost(auth.WithUserID(ctx, userID), &feedpb.UpdatePostRequest{
		PostId: id, Draft: draft.toRequest(),
	})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, viewFeedPost(post))
}

func (h *Handlers) handleCreateFeedPost(w http.ResponseWriter, r *http.Request) {
	var draft feedPostBody
	if !decodeFeedJSON(w, r, 96<<10, &draft) {
		return
	}

	ctx, cancel := h.callCtx(r)
	defer cancel()
	userID, _ := userIDFromContext(ctx)
	resp, err := h.Upstream.Feed.CreatePost(auth.WithUserID(ctx, userID), draft.toRequest())
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": resp.GetId()})
}

func (h *Handlers) handleListFeedPosts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ctx, cancel := h.callCtx(r)
	defer cancel()
	userID, _ := userIDFromContext(ctx)
	resp, err := h.Upstream.Feed.ListPosts(auth.WithUserID(ctx, userID), &feedpb.ListPostsRequest{
		Kind: q.Get("kind"), NewsStatus: q.Get("status"), Tickers: q["ticker"],
		MinRating: q.Get("min_rating"), MinNewsRating: q.Get("min_news_rating"),
		MinForecastRating: q.Get("min_forecast_rating"),
		Before:            q.Get("before"), BeforeAt: q.Get("before_at"),
	})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}

	posts := make([]feedPostView, 0, len(resp.GetPosts()))
	for _, post := range resp.GetPosts() {
		posts = append(posts, viewFeedPost(post))
	}
	var nextAt *time.Time
	if timestamp := resp.GetNextAt(); timestamp != nil {
		value := timestamp.AsTime()
		nextAt = &value
	}
	writeJSON(w, http.StatusOK, map[string]any{"posts": posts, "next": resp.Next, "next_at": nextAt})
}

func viewFeedPost(post *feedpb.Post) feedPostView {
	return feedPostView{
		feedPostBody: feedPostBody{
			Kind: post.GetKind(), Title: post.GetTitle(), Body: post.GetBody(), Tickers: post.GetTickers(),
			Board: post.GetBoard(), ChangePercent: post.ChangePercent,
			Recommendation: post.GetRecommendation(), TargetDate: post.GetTargetDate(),
		},
		ID: post.GetId(), Author: post.GetAuthor(), AuthorIsAdmin: post.AuthorIsAdmin, Rating: post.Rating, RatingSamples: post.GetRatingSamples(),
		Verified: post.GetVerified(), CreatedAt: post.GetCreatedAt().AsTime(),
	}
}
