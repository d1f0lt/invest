package httpapi

import (
	"net/http"
	"strings"
	"time"

	securitiesreaderpb "invest/backend/services/gateway/internal/securitiesreaderpb"
)

type priceView struct {
	SecID          string    `json:"secid"`
	Board          string    `json:"board"`
	ShortName      *string   `json:"short_name"`
	SecName        *string   `json:"sec_name"`
	ISIN           *string   `json:"isin"`
	Currency       *string   `json:"currency"`
	Last           *float64  `json:"last_price"`
	Open           *float64  `json:"open_price"`
	High           *float64  `json:"high_price"`
	Low            *float64  `json:"low_price"`
	ValueToday     *float64  `json:"value_today"`
	VolumeToday    *int64    `json:"volume_today"`
	TradingStatus  *string   `json:"trading_status"`
	MoexUpdateTime *string   `json:"moex_update_time"`
	CollectedAt    time.Time `json:"collected_at"`
}

func toPriceView(p *securitiesreaderpb.PriceView) priceView {
	return priceView{
		SecID: p.GetSecid(), Board: p.GetBoard(),
		ShortName: p.ShortName, SecName: p.SecName, ISIN: p.Isin, Currency: p.Currency,
		Last: p.LastPrice, Open: p.OpenPrice, High: p.HighPrice, Low: p.LowPrice,
		ValueToday: p.ValueToday, VolumeToday: p.VolumeToday,
		TradingStatus: p.TradingStatus, MoexUpdateTime: p.MoexUpdateTime,
		CollectedAt: p.GetCollectedAt().AsTime(),
	}
}

func (h *Handlers) handleGetPrices(w http.ResponseWriter, r *http.Request) {
	var tickers []string
	if raw := strings.TrimSpace(r.URL.Query().Get("tickers")); raw != "" {
		tickers = strings.Split(raw, ",")
	}

	ctx, cancel := h.callCtx(r)
	defer cancel()

	resp, err := h.Upstream.SecuritiesReader.GetPrices(ctx, &securitiesreaderpb.GetPricesRequest{Tickers: tickers})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}

	out := make([]priceView, 0, len(resp.GetPrices()))
	for _, p := range resp.GetPrices() {
		out = append(out, toPriceView(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count":  len(out),
		"prices": out,
	})
}
