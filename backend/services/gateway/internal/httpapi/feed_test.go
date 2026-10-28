package httpapi

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	"invest/backend/services/gateway/internal/auth"
	feedpb "invest/backend/services/gateway/internal/feedpb"
	"invest/backend/services/gateway/internal/upstream"
)

type testFeedServer struct {
	feedpb.UnimplementedFeedServiceServer
}

func (s *testFeedServer) ListPosts(ctx context.Context, req *feedpb.ListPostsRequest) (*feedpb.ListPostsResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	ids := md.Get(auth.UserIDKey)
	if len(ids) != 1 {
		return nil, status.Error(codes.Unauthenticated, "expected one user id")
	}
	admin := true
	post := &feedpb.Post{
		AuthorIsAdmin: &admin,
		Kind:          req.GetKind(), Tickers: req.GetTickers(),
		Title: req.GetMinNewsRating(), Body: req.GetMinForecastRating(),
		Author: ids[0], CreatedAt: timestamppb.Now(),
	}
	next := int64(42)
	return &feedpb.ListPostsResponse{Posts: []*feedpb.Post{post}, Next: &next, NextAt: post.CreatedAt}, nil
}

func TestFeedListThroughGRPC(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	feedpb.RegisterFeedServiceServer(server, &testFeedServer{})
	go server.Serve(listener)
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient("passthrough:///feed", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	key := []byte("test-only-key-for-feed-route-check")
	userID := "11111111-1111-1111-1111-111111111111"
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject: userID, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handlers{
		JWTSecret: key, UpstreamTimeout: 3 * time.Second,
		Upstream: &upstream.Clients{Feed: feedpb.NewFeedServiceClient(conn)},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/feed?kind=all&ticker=SBER&ticker=MOEX&min_news_rating=50&min_forecast_rating=95", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set(auth.UserIDKey, "untrusted-header")
	rec := httptest.NewRecorder()
	h.requireAuth(h.handleListFeedPosts)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var body struct {
		Posts  []feedPostView `json:"posts"`
		Next   int64          `json:"next"`
		NextAt time.Time      `json:"next_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Next != 42 || body.NextAt.IsZero() || len(body.Posts) != 1 {
		t.Fatalf("unexpected response: %+v", body)
	}
	post := body.Posts[0]
	if post.AuthorIsAdmin == nil || !*post.AuthorIsAdmin || post.Author != userID || post.Kind != "all" || post.Title != "50" || post.Body != "95" ||
		len(post.Tickers) != 2 || post.Tickers[0] != "SBER" || post.Tickers[1] != "MOEX" {
		t.Fatalf("unexpected forwarded request: %+v", post)
	}
}
