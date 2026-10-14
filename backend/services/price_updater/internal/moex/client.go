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
	idx := columnIndex(resp.Description.Columns)
	for _, column := range []string{"name", "value"} {
		if _, ok := idx[column]; !ok {
			return nil, fmt.Errorf("moex description: missing column %s", column)
		}
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
