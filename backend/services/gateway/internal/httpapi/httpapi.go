package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"invest/backend/services/gateway/internal/upstream"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type Handlers struct {
	JWTSecret []byte

	Upstream *upstream.Clients

	UpstreamTimeout time.Duration

	Queue Pinger
	Store Pinger

	Reports *ReportsHandler

	Log *slog.Logger
}

func (h *Handlers) callCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), h.UpstreamTimeout)
}

func NewMux(h *Handlers) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.healthz)

	mux.HandleFunc("POST /api/v1/users", h.handleRegister)
	mux.HandleFunc("POST /api/v1/login", h.handleLogin)
	mux.HandleFunc("POST /api/v1/refresh", h.handleRefresh)
	mux.HandleFunc("POST /api/v1/logout", h.handleLogout)

	mux.HandleFunc("GET /api/v1/me", h.requireAuth(h.handleMe))
	mux.HandleFunc("GET /api/v1/users/{id}", h.requireAuth(h.handleGetUser))

	mux.HandleFunc("GET /api/v1/prices", h.requireAuth(h.handleGetPrices))
	mux.HandleFunc("GET /api/v1/prices/{secid}/candles", h.requireAuth(h.handleGetCandles))
	mux.HandleFunc("GET /api/v1/securities", h.requireAuth(h.handleSearchSecurities))
	mux.HandleFunc("GET /api/v1/securities/{secid}/info", h.requireAuth(h.handleGetSecurityInfo))
	mux.HandleFunc("GET /api/v1/securities/{secid}/dividends", h.requireAuth(h.handleGetDividends))

	mux.HandleFunc("POST /api/v1/portfolios", h.requireAuth(h.handleCreatePortfolio))
	mux.HandleFunc("GET /api/v1/portfolios", h.requireAuth(h.handleListPortfolios))
	mux.HandleFunc("GET /api/v1/portfolios/{id}", h.requireAuth(h.handleGetPortfolio))
	mux.HandleFunc("PATCH /api/v1/portfolios/{id}", h.requireAuth(h.handleUpdatePortfolio))
	mux.HandleFunc("POST /api/v1/portfolios/{id}/trades", h.requireAuth(h.handleCreateTrade))
	mux.HandleFunc("GET /api/v1/portfolios/{id}/trades", h.requireAuth(h.handleListTrades))
	mux.HandleFunc("GET /api/v1/portfolios/{id}/holdings", h.requireAuth(h.handleGetHoldings))
	mux.HandleFunc("GET /api/v1/portfolios/{id}/pnl", h.requireAuth(h.handleGetPnL))
	mux.HandleFunc("GET /api/v1/portfolios/{id}/cash-operations", h.requireAuth(h.handleListCashOperations))

	mux.HandleFunc("GET /api/v1/brokers", h.requireAuth(h.handleListBrokers))
	mux.HandleFunc("GET /static/brokers/{file}", handleBrokerIcon)

	mux.HandleFunc("POST /api/v1/portfolios/{id}/reports", h.requireAuth(h.Reports.Upload))
	mux.HandleFunc("GET /api/v1/portfolios/{id}/reports", h.requireAuth(h.Reports.List))
	mux.HandleFunc("GET /api/v1/portfolios/{id}/reports/{report_id}", h.requireAuth(h.Reports.Get))

	return mux
}

func (h *Handlers) healthz(w http.ResponseWriter, r *http.Request) {
	problems := make(map[string]string)

	ctx, cancel := h.callCtx(r)
	defer cancel()
	for name, err := range h.Upstream.CheckAll(ctx) {
		problems[name] = err
	}
	if err := h.Queue.Ping(r.Context()); err != nil {
		problems["rabbitmq"] = err.Error()
	}
	if err := h.Store.Ping(r.Context()); err != nil {
		problems["minio"] = err.Error()
	}

	if len(problems) > 0 {
		writeJSON(w, http.StatusServiceUnavailable, problems)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, errorResponse{Error: message})
}

func writeUpstreamError(w http.ResponseWriter, log *slog.Logger, err error) {
	st, ok := status.FromError(err)
	if !ok {
		log.Error("non-gRPC error from upstream call", "error", err)
		writeError(w, http.StatusBadGateway, "upstream service unavailable")
		return
	}

	switch st.Code() {
	case codes.InvalidArgument:
		writeError(w, http.StatusBadRequest, st.Message())
	case codes.Unauthenticated:

		writeError(w, http.StatusUnauthorized, st.Message())
	case codes.NotFound:
		writeError(w, http.StatusNotFound, st.Message())
	case codes.AlreadyExists:
		writeError(w, http.StatusConflict, st.Message())
	case codes.FailedPrecondition:

		writeError(w, http.StatusUnprocessableEntity, st.Message())
	case codes.Unavailable:

		log.Error("upstream unavailable", "error", err)
		writeError(w, http.StatusBadGateway, "upstream service unavailable")
	default:
		log.Error("upstream call failed", "code", st.Code(), "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
