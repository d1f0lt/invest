package httpapi

import (
	"net/http"
	"strings"
	"time"

	"invest/backend/services/gateway/internal/auth"
	portfoliopb "invest/backend/services/gateway/internal/portfoliopb"
)

var valueHistoryRanges = map[string]bool{"week": true, "month": true, "year": true, "all": true}

type valuePointView struct {
	Date        time.Time `json:"date"`
	Value       float64   `json:"value"`
	NetDeposits float64   `json:"net_deposits"`
}

type valueHistoryResponse struct {
	Range  string           `json:"range"`
	Points []valuePointView `json:"points"`
}

func (h *Handlers) handleGetValueHistory(w http.ResponseWriter, r *http.Request) {
	rangeName := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("range")))
	if rangeName == "" {
		rangeName = "all"
	}
	if !valueHistoryRanges[rangeName] {
		writeError(w, http.StatusBadRequest, "range must be one of: week, month, year, all")
		return
	}

	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	resp, err := h.Upstream.Portfolio.GetValueHistory(ctx, &portfoliopb.GetValueHistoryRequest{
		PortfolioId: r.PathValue("id"),
		Range:       rangeName,
	})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}

	out := valueHistoryResponse{Range: rangeName, Points: make([]valuePointView, 0, len(resp.GetPoints()))}
	for _, p := range resp.GetPoints() {
		out.Points = append(out.Points, valuePointView{
			Date:        p.GetDate().AsTime(),
			Value:       p.GetValue(),
			NetDeposits: p.GetNetDeposits(),
		})
	}
	writeJSON(w, http.StatusOK, out)
}
