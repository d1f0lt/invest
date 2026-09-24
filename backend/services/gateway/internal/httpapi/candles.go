package httpapi

import (
	"net/http"
	"strings"
	"time"

	pricereaderpb "invest/backend/services/gateway/internal/pricereaderpb"
)

var candleRanges = map[string]pricereaderpb.CandleRange{
	"day":   pricereaderpb.CandleRange_CANDLE_RANGE_DAY,
	"week":  pricereaderpb.CandleRange_CANDLE_RANGE_WEEK,
	"month": pricereaderpb.CandleRange_CANDLE_RANGE_MONTH,
	"year":  pricereaderpb.CandleRange_CANDLE_RANGE_YEAR,
	"all":   pricereaderpb.CandleRange_CANDLE_RANGE_ALL,
}

var candleIntervalNames = map[pricereaderpb.CandleInterval]string{
	pricereaderpb.CandleInterval_CANDLE_INTERVAL_HOUR: "hour",
	pricereaderpb.CandleInterval_CANDLE_INTERVAL_DAY:  "day",
	pricereaderpb.CandleInterval_CANDLE_INTERVAL_WEEK: "week",
}

type candleView struct {
	Start  time.Time `json:"start"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume *int64    `json:"volume"`
	Value  *float64  `json:"value"`
}

type candlesResponse struct {
	SecID    string       `json:"secid"`
	Board    string       `json:"board"`
	Range    string       `json:"range"`
	Interval string       `json:"interval"`
	Count    int          `json:"count"`
	Candles  []candleView `json:"candles"`
}

func (h *Handlers) handleGetCandles(w http.ResponseWriter, r *http.Request) {
	rangeName := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("range")))
	if rangeName == "" {
		rangeName = "day"
	}
	rng, ok := candleRanges[rangeName]
	if !ok {
		writeError(w, http.StatusBadRequest, "range must be one of: day, week, month, year, all")
		return
	}

	ctx, cancel := h.callCtx(r)
	defer cancel()

	resp, err := h.Upstream.PriceReader.GetCandles(ctx, &pricereaderpb.GetCandlesRequest{
		Secid: r.PathValue("secid"),
		Board: r.URL.Query().Get("board"),
		Range: rng,
	})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}

	writeJSON(w, http.StatusOK, toCandlesResponse(rangeName, resp))
}

func toCandlesResponse(rangeName string, resp *pricereaderpb.GetCandlesResponse) candlesResponse {
	out := candlesResponse{
		SecID:    resp.GetSecid(),
		Board:    resp.GetBoard(),
		Range:    rangeName,
		Interval: candleIntervalNames[resp.GetInterval()],
		Candles:  make([]candleView, 0, len(resp.GetCandles())),
	}
	for _, c := range resp.GetCandles() {
		out.Candles = append(out.Candles, candleView{
			Start: c.GetStart().AsTime(),
			Open:  c.GetOpen(), High: c.GetHigh(), Low: c.GetLow(), Close: c.GetClose(),
			Volume: c.Volume, Value: c.Value,
		})
	}
	out.Count = len(out.Candles)
	return out
}
