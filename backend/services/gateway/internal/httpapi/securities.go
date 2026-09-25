package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	securitiesreaderpb "invest/backend/services/gateway/internal/securitiesreaderpb"
)




func (h *Handlers) handleSearchSecurities(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}
	var limit int32
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = int32(n)
	}

	ctx, cancel := h.callCtx(r)
	defer cancel()

	resp, err := h.Upstream.SecuritiesReader.SearchSecurities(ctx, &securitiesreaderpb.SearchSecuritiesRequest{
		Query: query,
		Limit: limit,
	})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}

	out := make([]priceView, 0, len(resp.GetSecurities()))
	for _, p := range resp.GetSecurities() {
		out = append(out, toPriceView(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count":      len(out),
		"securities": out,
	})
}
