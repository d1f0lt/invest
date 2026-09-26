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

type securityInfoField struct {
	Name  string  `json:"name"`
	Title string  `json:"title"`
	Value string  `json:"value"`
	Type  string  `json:"type"`
	Unit  *string `json:"unit"`
}

type securityInfoResponse struct {
	SecID  string              `json:"secid"`
	Board  string              `json:"board"`
	Fields []securityInfoField `json:"fields"`
}

func (h *Handlers) handleGetSecurityInfo(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := h.callCtx(r)
	defer cancel()

	resp, err := h.Upstream.SecuritiesReader.GetSecurityInfo(ctx, &securitiesreaderpb.GetSecurityInfoRequest{
		Secid: r.PathValue("secid"),
		Board: r.URL.Query().Get("board"),
	})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}

	out := securityInfoResponse{
		SecID:  resp.GetSecid(),
		Board:  resp.GetBoard(),
		Fields: make([]securityInfoField, 0, len(resp.GetFields())),
	}
	for _, f := range resp.GetFields() {
		out.Fields = append(out.Fields, securityInfoField{
			Name: f.GetName(), Title: f.GetTitle(), Value: f.GetValue(), Type: f.GetType(), Unit: f.Unit,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type dividendView struct {
	RegistryCloseDate string  `json:"registry_close_date"`
	Value             float64 `json:"value"`
	Currency          string  `json:"currency"`
	Forecast          bool    `json:"forecast"`
	DeclaredDate      *string `json:"declared_date"`
}

func (h *Handlers) handleGetDividends(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := h.callCtx(r)
	defer cancel()

	resp, err := h.Upstream.SecuritiesReader.GetDividends(ctx, &securitiesreaderpb.GetDividendsRequest{
		Secid: r.PathValue("secid"),
	})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}

	out := make([]dividendView, 0, len(resp.GetDividends()))
	for _, d := range resp.GetDividends() {
		out = append(out, dividendView{
			RegistryCloseDate: d.GetRegistryCloseDate(),
			Value:             d.GetValue(),
			Currency:          d.GetCurrency(),
			Forecast:          d.GetForecast(),
			DeclaredDate:      nonEmpty(d.GetDeclaredDate()),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"secid":     resp.GetSecid(),
		"count":     len(out),
		"dividends": out,
	})
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
