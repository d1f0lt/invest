package httpapi

import (
	"net/http"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"invest/backend/services/gateway/internal/auth"
	portfoliopb "invest/backend/services/gateway/internal/portfoliopb"
)

type createPortfolioRequest struct {
	Name string `json:"name,omitempty"`
}

type portfolioResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toPortfolioResponse(p *portfoliopb.Portfolio) portfolioResponse {
	return portfolioResponse{
		ID:        p.GetId(),
		Name:      p.GetName(),
		CreatedAt: p.GetCreatedAt().AsTime(),
		UpdatedAt: p.GetUpdatedAt().AsTime(),
	}
}

type createTradeRequest struct {
	SecID      string     `json:"secid"`
	Board      string     `json:"board"`
	Side       string     `json:"side"`
	Quantity   float64    `json:"quantity"`
	Price      float64    `json:"price"`
	Fee        float64    `json:"fee,omitempty"`
	Currency   string     `json:"currency,omitempty"`
	ExecutedAt *time.Time `json:"executed_at,omitempty"`
}

type tradeResponse struct {
	ID         string    `json:"id"`
	SecID      string    `json:"secid"`
	Board      string    `json:"board"`
	Side       string    `json:"side"`
	Quantity   float64   `json:"quantity"`
	Price      float64   `json:"price"`
	Fee        float64   `json:"fee"`
	Currency   string    `json:"currency"`
	ExecutedAt time.Time `json:"executed_at"`
}

func toTradeResponse(t *portfoliopb.Trade) tradeResponse {
	return tradeResponse{
		ID: t.GetId(), SecID: t.GetSecid(), Board: t.GetBoard(), Side: t.GetSide(),
		Quantity: t.GetQuantity(), Price: t.GetPrice(), Fee: t.GetFee(),
		Currency: t.GetCurrency(), ExecutedAt: t.GetExecutedAt().AsTime(),
	}
}

type holdingResponse struct {
	SecID         string   `json:"secid"`
	Board         string   `json:"board"`
	Quantity      float64  `json:"quantity"`
	AvgCost       float64  `json:"avg_cost"`
	CurrentPrice  *float64 `json:"current_price,omitempty"`
	MarketValue   *float64 `json:"market_value,omitempty"`
	UnrealizedPnL *float64 `json:"unrealized_pnl,omitempty"`
}

func toHoldingResponse(h *portfoliopb.Holding) holdingResponse {
	return holdingResponse{
		SecID: h.GetSecid(), Board: h.GetBoard(), Quantity: h.GetQuantity(), AvgCost: h.GetAvgCost(),
		CurrentPrice: h.CurrentPrice, MarketValue: h.MarketValue, UnrealizedPnL: h.UnrealizedPnl,
	}
}

type instrumentPnLResponse struct {
	holdingResponse
	RealizedPnL float64 `json:"realized_pnl"`
	TotalPnL    float64 `json:"total_pnl"`
}

func toInstrumentResponse(i *portfoliopb.InstrumentPnL) instrumentPnLResponse {
	return instrumentPnLResponse{
		holdingResponse: toHoldingResponse(i.GetHolding()),
		RealizedPnL:     i.GetRealizedPnl(),
		TotalPnL:        i.GetTotalPnl(),
	}
}

type pnlSummaryResponse struct {
	Instruments        []instrumentPnLResponse `json:"instruments"`
	TotalRealizedPnL   float64                 `json:"total_realized_pnl"`
	TotalUnrealizedPnL float64                 `json:"total_unrealized_pnl"`
	TotalPnL           float64                 `json:"total_pnl"`
}

func (h *Handlers) handleCreatePortfolio(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	var req createPortfolioRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	p, err := h.Upstream.Portfolio.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: req.Name})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, toPortfolioResponse(p))
}

func (h *Handlers) handleListPortfolios(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	resp, err := h.Upstream.Portfolio.ListPortfolios(ctx, &emptypb.Empty{})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	out := make([]portfolioResponse, 0, len(resp.GetPortfolios()))
	for _, p := range resp.GetPortfolios() {
		out = append(out, toPortfolioResponse(p))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) handleGetPortfolio(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	p, err := h.Upstream.Portfolio.GetPortfolio(ctx, &portfoliopb.GetPortfolioRequest{Id: r.PathValue("id")})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, toPortfolioResponse(p))
}

func (h *Handlers) handleCreateTrade(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	var req createTradeRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	pbReq := &portfoliopb.CreateTradeRequest{
		PortfolioId: r.PathValue("id"),
		Secid:       req.SecID,
		Board:       req.Board,
		Side:        req.Side,
		Quantity:    req.Quantity,
		Price:       req.Price,
		Fee:         req.Fee,
		Currency:    req.Currency,
	}
	if req.ExecutedAt != nil {
		pbReq.ExecutedAt = timestamppb.New(*req.ExecutedAt)
	}

	trade, err := h.Upstream.Portfolio.CreateTrade(ctx, pbReq)
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, toTradeResponse(trade))
}

func (h *Handlers) handleListTrades(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	resp, err := h.Upstream.Portfolio.ListTrades(ctx, &portfoliopb.ListTradesRequest{PortfolioId: r.PathValue("id")})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	out := make([]tradeResponse, 0, len(resp.GetTrades()))
	for _, t := range resp.GetTrades() {
		out = append(out, toTradeResponse(t))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) handleGetHoldings(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	resp, err := h.Upstream.Portfolio.GetHoldings(ctx, &portfoliopb.GetHoldingsRequest{PortfolioId: r.PathValue("id")})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	out := make([]holdingResponse, 0, len(resp.GetHoldings()))
	for _, hv := range resp.GetHoldings() {
		out = append(out, toHoldingResponse(hv))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) handleGetPnL(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	summary, err := h.Upstream.Portfolio.GetPnL(ctx, &portfoliopb.GetPnLRequest{PortfolioId: r.PathValue("id")})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	instruments := make([]instrumentPnLResponse, 0, len(summary.GetInstruments()))
	for _, i := range summary.GetInstruments() {
		instruments = append(instruments, toInstrumentResponse(i))
	}
	writeJSON(w, http.StatusOK, pnlSummaryResponse{
		Instruments:        instruments,
		TotalRealizedPnL:   summary.GetTotalRealizedPnl(),
		TotalUnrealizedPnL: summary.GetTotalUnrealizedPnl(),
		TotalPnL:           summary.GetTotalPnl(),
	})
}
