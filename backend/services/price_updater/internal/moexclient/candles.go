package moexclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

func (c *Client) FetchIntradayCandles(ctx context.Context, secid, board string, day time.Time, loc *time.Location) ([]DailyCandle, error) {
	var out []DailyCandle
	for start := 0; ; {
		address := fmt.Sprintf("%s/engines/stock/markets/%s/boards/%s/securities/%s/candles.json?iss.meta=off&interval=10&from=%s&till=%s&start=%d", c.baseURL, MarketFor(board), url.PathEscape(board), url.PathEscape(secid), day.In(loc).Format("2006-01-02"), day.In(loc).Format("2006-01-02"), start)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		var page struct {
			Candles issTable `json:"candles"`
		}
		err = func() error {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("candles %s/%s: status %d", board, secid, resp.StatusCode)
			}
			return json.NewDecoder(resp.Body).Decode(&page)
		}()
		if err != nil {
			return nil, err
		}
		if len(page.Candles.Data) == 0 {
			return out, nil
		}
		idx := columnIndex(page.Candles.Columns)
		for _, field := range []string{"begin", "open", "high", "low", "close"} {
			if _, ok := idx[field]; !ok {
				return nil, fmt.Errorf("candles: missing column %s", field)
			}
		}
		for _, row := range page.Candles.Data {
			at, err := time.ParseInLocation("2006-01-02 15:04:05", strAt(row, idx, "begin"), loc)
			if err != nil {
				return nil, err
			}
			open, high, low, close := floatPtrAt(row, idx, "open"), floatPtrAt(row, idx, "high"), floatPtrAt(row, idx, "low"), floatPtrAt(row, idx, "close")
			if open == nil || high == nil || low == nil || close == nil {
				continue
			}
			out = append(out, DailyCandle{SecID: secid, BoardID: board, TradeDate: at, Open: *open, High: *high, Low: *low, Close: *close, Volume: intPtrAt(row, idx, "volume"), Value: floatPtrAt(row, idx, "value")})
		}
		start += len(page.Candles.Data)
	}
}
