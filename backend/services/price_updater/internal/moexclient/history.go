package moexclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"
)

type DailyCandle struct {
	SecID     string
	BoardID   string
	TradeDate time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    *int64
	Value     *float64
}

type historyResponse struct {
	History issTable `json:"history"`
	Cursor  issTable `json:"history.cursor"`
}

const historyPageSize = 100

func (c *Client) FetchBoardHistory(ctx context.Context, board string, date time.Time) ([]DailyCandle, error) {
	market := MarketFor(board)
	day := date.Format("2006-01-02")

	var rows []historyRow
	for start := 0; ; {
		url := fmt.Sprintf("%s/history/engines/stock/markets/%s/boards/%s/securities.json?iss.meta=off&date=%s&start=%d&limit=%d",
			c.baseURL, market, board, day, start, historyPageSize)

		page, err := c.getHistoryPage(ctx, url)
		if err != nil {
			return nil, fmt.Errorf("history %s %s: %w", board, day, err)
		}

		idx := columnIndex(page.History.Columns)
		for _, r := range page.History.Data {
			rows = append(rows, parseHistoryRow(r, idx))
		}

		total, hasCursor := cursorTotal(page.Cursor)
		start += len(page.History.Data)
		if len(page.History.Data) == 0 {
			break
		}
		if hasCursor {
			if start >= total {
				break
			}
		} else if len(page.History.Data) < historyPageSize {
			break
		}
	}

	return mergeSessions(rows, board), nil
}

func (c *Client) getHistoryPage(ctx context.Context, url string) (historyResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return historyResponse{}, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return historyResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return historyResponse{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var parsed historyResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return historyResponse{}, fmt.Errorf("decode: %w", err)
	}
	return parsed, nil
}

func cursorTotal(t issTable) (int, bool) {
	if len(t.Data) == 0 {
		return 0, false
	}
	idx := columnIndex(t.Columns)
	p := intPtrAt(t.Data[0], idx, "TOTAL")
	if p == nil {
		return 0, false
	}
	return int(*p), true
}

type historyRow struct {
	secID     string
	boardID   string
	tradeDate string
	session   int
	open      *float64
	high      *float64
	low       *float64
	close     *float64
	volume    *int64
	value     *float64
}

func parseHistoryRow(r []interface{}, idx map[string]int) historyRow {
	return historyRow{
		secID:     strAt(r, idx, "SECID"),
		boardID:   strAt(r, idx, "BOARDID"),
		tradeDate: strAt(r, idx, "TRADEDATE"),
		session:   sessionAt(r, idx),
		open:      floatPtrAt(r, idx, "OPEN"),
		high:      floatPtrAt(r, idx, "HIGH"),
		low:       floatPtrAt(r, idx, "LOW"),
		close:     floatPtrAt(r, idx, "CLOSE"),
		volume:    intPtrAt(r, idx, "VOLUME"),
		value:     floatPtrAt(r, idx, "VALUE"),
	}
}

func sessionAt(r []interface{}, idx map[string]int) int {
	switch v := cell(r, idx, "TRADINGSESSION").(type) {
	case float64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return -1
}

const sessionTotal = 3

func mergeSessions(rows []historyRow, board string) []DailyCandle {
	bySec := map[string][]historyRow{}
	var order []string
	for _, r := range rows {
		if r.secID == "" || r.open == nil || r.close == nil {
			continue
		}
		if _, ok := bySec[r.secID]; !ok {
			order = append(order, r.secID)
		}
		bySec[r.secID] = append(bySec[r.secID], r)
	}

	out := make([]DailyCandle, 0, len(order))
	for _, secID := range order {
		group := bySec[secID]
		for _, r := range group {
			if r.session == sessionTotal {
				group = []historyRow{r}
				break
			}
		}
		sort.SliceStable(group, func(i, j int) bool { return group[i].session < group[j].session })

		first, last := group[0], group[len(group)-1]
		date, err := time.Parse("2006-01-02", first.tradeDate)
		if err != nil {
			continue
		}
		boardID := first.boardID
		if boardID == "" {
			boardID = board
		}

		c := DailyCandle{
			SecID:     secID,
			BoardID:   boardID,
			TradeDate: date,
			Open:      *first.open,
			Close:     *last.close,
			High:      *first.open,
			Low:       *first.open,
		}
		for _, r := range group {
			for _, p := range []*float64{r.open, r.close, r.high, r.low} {
				if p == nil {
					continue
				}
				if *p > c.High {
					c.High = *p
				}
				if *p < c.Low {
					c.Low = *p
				}
			}
			if r.volume != nil {
				v := *r.volume
				if c.Volume != nil {
					v += *c.Volume
				}
				c.Volume = &v
			}
			if r.value != nil {
				v := *r.value
				if c.Value != nil {
					v += *c.Value
				}
				c.Value = &v
			}
		}
		out = append(out, c)
	}
	return out
}
