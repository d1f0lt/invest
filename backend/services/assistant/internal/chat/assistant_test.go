package chat

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	feedpb "invest/backend/services/assistant/internal/feedpb"
	portfoliopb "invest/backend/services/assistant/internal/portfoliopb"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type testPortfolio struct {
	portfoliopb.PortfolioServiceClient
	calls int
	t     *testing.T
}

func (p *testPortfolio) GetHoldings(ctx context.Context, req *portfoliopb.GetHoldingsRequest, _ ...grpc.CallOption) (*portfoliopb.GetHoldingsResponse, error) {
	p.calls++
	assertUserID(p.t, ctx)
	if req.GetPortfolioId() != "mine" {
		return nil, status.Error(codes.NotFound, "portfolio not found")
	}
	return &portfoliopb.GetHoldingsResponse{Holdings: []*portfoliopb.Holding{{Secid: "SBER", Quantity: 5}}}, nil
}

type testFeed struct {
	feedpb.FeedServiceClient
	kinds []string
	t     *testing.T
}

func (f *testFeed) ListPosts(ctx context.Context, req *feedpb.ListPostsRequest, _ ...grpc.CallOption) (*feedpb.ListPostsResponse, error) {
	assertUserID(f.t, ctx)
	if !reflect.DeepEqual(req.GetTickers(), []string{"SBER"}) {
		f.t.Errorf("unexpected ticker filter: %v", req.GetTickers())
	}
	f.kinds = append(f.kinds, req.GetKind())
	rating := int32(87)
	return &feedpb.ListPostsResponse{Posts: []*feedpb.Post{{
		Id: int64(len(f.kinds)), Kind: req.GetKind(), Tickers: req.GetTickers(),
		Author: "Автор", Body: "Текст публикации", Rating: &rating, RatingSamples: 12,
		Verified: req.GetKind() == "news", CreatedAt: timestamppb.Now(),
	}}}, nil
}

func assertUserID(t *testing.T, ctx context.Context) {
	t.Helper()
	md, _ := metadata.FromOutgoingContext(ctx)
	if ids := md.Get("x-user-id"); !reflect.DeepEqual(ids, []string{"test-user"}) {
		t.Errorf("unexpected user ID metadata: %v", ids)
	}
}

func TestAssistantGroundedChat(t *testing.T) {
	portfolio := &testPortfolio{t: t}
	feed := &testFeed{t: t}
	modelCalls := 0
	h := &Handler{
		JWTSecret: []byte("test-only-assistant-key-at-least-32-bytes"),
		Portfolio: portfolio, Feed: feed, Log: slog.Default(),
		Model: &AssistantModel{
			endpoint: "http://model.test/v1/chat/completions", name: "test-model", key: "test-key",
			client: &http.Client{Transport: testTransport(func(req *http.Request) (*http.Response, error) {
				modelCalls++
				if req.Header.Get("Authorization") != "Bearer test-key" {
					t.Error("model authorization was not set")
				}
				var request struct {
					Messages   []assistantMessage `json:"messages"`
					ToolChoice string             `json:"tool_choice"`
				}
				if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				last := request.Messages[len(request.Messages)-1]
				firstResponse := `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[
					{"id":"call-news","type":"function","function":{"name":"search_feed","arguments":"{\"ticker\":\"sber\",\"kind\":\"news\"}"}},
					{"id":"call-forecast","type":"function","function":{"name":"search_feed","arguments":"{\"ticker\":\"SBER\",\"kind\":\"forecast\"}"}}
				]}}]}`
				finalResponse := `{"choices":[{"message":{"role":"assistant","content":"Новости [1], прогноз [2]."}}]}`
				body := finalResponse
				switch modelCalls {
				case 1:
					if last.Content == nil || !strings.Contains(*last.Content, `"quantity":5`) || request.ToolChoice != "auto" {
						t.Errorf("missing holdings or tools: %+v", request)
					}
					body = firstResponse
				case 2:
					for i, id := range []string{"call-news", "call-forecast"} {
						result := request.Messages[len(request.Messages)-2+i]
						if result.Role != "tool" || result.ToolCallID != id || result.Content == nil {
							t.Fatalf("missing tool result: %+v", result)
						}
						var posts []feedPostView
						if err := json.Unmarshal([]byte(*result.Content), &posts); err != nil ||
							len(posts) != 1 || posts[0].Rating == nil || *posts[0].Rating != 87 || posts[0].RatingSamples != 12 {
							t.Errorf("missing author score: %s", *result.Content)
						}
					}
				case 3:
					if last.Content == nil || *last.Content != "Вопрос без портфеля" {
						t.Errorf("unexpected question without portfolio: %+v", last)
					}
				default:
					t.Error("unexpected model call")
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			})},
		},
	}

	signedToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject: "test-user", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}).SignedString(h.JWTSecret)
	if err != nil {
		t.Fatal(err)
	}

	invoke := func(body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/assistant/chat", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-User-ID", "other-user")
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, req)
		return recorder
	}

	if result := invoke(`{"question":"Вопрос","portfolio_id":"mine"}`, "invalid"); result.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token: got %d %s", result.Code, result.Body)
	}
	if result := invoke(`{"question":"Вопрос","portfolio_id":"other"}`, signedToken); result.Code != http.StatusNotFound || modelCalls != 0 {
		t.Fatalf("foreign portfolio: got %d %s, model called %d times", result.Code, result.Body, modelCalls)
	}

	result := invoke(`{"question":"Вопрос","portfolio_id":"mine"}`, signedToken)
	if result.Code != http.StatusOK {
		t.Fatalf("response: got %d %s", result.Code, result.Body)
	}
	var answer assistantChatResponse
	if err := json.Unmarshal(result.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Answer != "Новости [1], прогноз [2]." || len(answer.Sources) != 2 || !answer.Sources[0].Verified ||
		!reflect.DeepEqual(feed.kinds, []string{"news", "forecast"}) {
		t.Fatalf("unexpected grounded response: %+v, lookups %v", answer, feed.kinds)
	}
	if result := invoke(`{"question":"Вопрос без портфеля"}`, signedToken); result.Code != http.StatusOK || portfolio.calls != 2 || modelCalls != 3 {
		t.Fatalf("unattached question: got %d %s, portfolio calls = %d, model calls = %d", result.Code, result.Body, portfolio.calls, modelCalls)
	}
}
