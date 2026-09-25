package moex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const pageSize = 500

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
	}
}

type Candle struct {
	Begin time.Time
	Open  float64
	High  float64
	Low   float64
	Close float64
}

type DescriptionField struct {
	Name   string
	Title  string
	Value  string
	Type   string
	Hidden bool
}

type issTable struct {
	Columns []string            `json:"columns"`
	Data    [][]json.RawMessage `json:"data"`
}

var bondBoards = map[string]bool{
	"TQOB": true,
	"TQCB": true,
	"TQIR": true,
	"TQOD": true,
	"TQOE": true,
	"TQOY": true,
	"TQRD": true,
}

func MarketFor(board string) string {
	if bondBoards[board] {
		return "bonds"
	}
	return "shares"
}

func (c *Client) IntradayCandles(ctx context.Context, secid, board string, day time.Time, loc *time.Location) ([]Candle, error) {
	return c.Candles(ctx, secid, board, 10, day, day, loc)
}

func (c *Client) Candles(ctx context.Context, secid, board string, interval int, from, till time.Time, loc *time.Location) ([]Candle, error) {
	path := fmt.Sprintf("/engines/stock/markets/%s/boards/%s/securities/%s/candles.json",
		MarketFor(board), url.PathEscape(board), url.PathEscape(secid))

	var out []Candle
	for start := 0; ; start += pageSize {
		q := url.Values{}
		q.Set("iss.meta", "off")
		q.Set("interval", strconv.Itoa(interval))
		q.Set("from", from.In(loc).Format("2006-01-02"))
		q.Set("till", till.In(loc).Format("2006-01-02"))
		q.Set("start", strconv.Itoa(start))

		var resp struct {
			Candles issTable `json:"candles"`
		}
		if err := c.getJSON(ctx, path, q, &resp); err != nil {
			return nil, err
		}
		page, err := parseCandles(resp.Candles, loc)
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(resp.Candles.Data) < pageSize || start >= 10*pageSize {
			return out, nil
		}
	}
}

func parseCandles(t issTable, loc *time.Location) ([]Candle, error) {
	idx := columnIndex(t.Columns)
	for _, col := range []string{"open", "close", "high", "low", "begin"} {
		if _, ok := idx[col]; !ok {
			return nil, fmt.Errorf("moex candles: column %q missing", col)
		}
	}
	out := make([]Candle, 0, len(t.Data))
	for _, row := range t.Data {
		begin, err := time.ParseInLocation("2006-01-02 15:04:05", cellString(row, pos(idx, "begin")), loc)
		if err != nil {
			return nil, fmt.Errorf("moex candles: bad begin: %w", err)
		}
		open, ok1 := cellFloat(row, pos(idx, "open"))
		high, ok2 := cellFloat(row, pos(idx, "high"))
		low, ok3 := cellFloat(row, pos(idx, "low"))
		cls, ok4 := cellFloat(row, pos(idx, "close"))
		if !ok1 || !ok2 || !ok3 || !ok4 {
			continue
		}
		out = append(out, Candle{Begin: begin, Open: open, High: high, Low: low, Close: cls})
	}
	return out, nil
}

func (c *Client) Description(ctx context.Context, secid string) ([]DescriptionField, error) {
	q := url.Values{}
	q.Set("iss.meta", "off")
	q.Set("iss.only", "description")
	var resp struct {
		Description issTable `json:"description"`
	}
	if err := c.getJSON(ctx, "/securities/"+url.PathEscape(secid)+".json", q, &resp); err != nil {
		return nil, err
	}
	return parseDescription(resp.Description), nil
}

func parseDescription(t issTable) []DescriptionField {
	idx := columnIndex(t.Columns)
	out := make([]DescriptionField, 0, len(t.Data))
	for _, row := range t.Data {
		name := cellString(row, pos(idx, "name"))
		if name == "" {
			continue
		}
		hidden, _ := cellFloat(row, pos(idx, "is_hidden"))
		out = append(out, DescriptionField{
			Name:   name,
			Title:  cellString(row, pos(idx, "title")),
			Value:  cellString(row, pos(idx, "value")),
			Type:   cellString(row, pos(idx, "type")),
			Hidden: hidden != 0,
		})
	}
	return out
}

func (c *Client) EmitterTitle(ctx context.Context, secid string) (string, error) {
	q := url.Values{}
	q.Set("iss.meta", "off")
	q.Set("iss.only", "securities")
	q.Set("q", secid)
	q.Set("securities.columns", "secid,emitent_title")
	var resp struct {
		Securities issTable `json:"securities"`
	}
	if err := c.getJSON(ctx, "/securities.json", q, &resp); err != nil {
		return "", err
	}
	idx := columnIndex(resp.Securities.Columns)
	for _, row := range resp.Securities.Data {
		if strings.EqualFold(cellString(row, pos(idx, "secid")), secid) {
			return cellString(row, pos(idx, "emitent_title")), nil
		}
	}
	return "", nil
}

func (c *Client) getJSON(ctx context.Context, path string, q url.Values, dst any) error {
	u := c.baseURL + path + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("moex: build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("moex: %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("moex: %s: status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("moex: %s: decode: %w", path, err)
	}
	return nil
}

func columnIndex(columns []string) map[string]int {
	idx := make(map[string]int, len(columns))
	for i, name := range columns {
		idx[strings.ToLower(name)] = i
	}
	return idx
}

func pos(idx map[string]int, name string) int {
	if i, ok := idx[name]; ok {
		return i
	}
	return -1
}

func cellString(row []json.RawMessage, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	raw := row[i]
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(string(raw))
}

func cellFloat(row []json.RawMessage, i int) (float64, bool) {
	s := cellString(row, i)
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}
