package moexclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

type Security struct {
	SecID     string
	BoardID   string
	ShortName string
	SecName   string
	ISIN      string
	Currency  string
	LotSize   int64
	Decimals  int

	
	
	
	FaceValue      *float64
	PriceInPercent bool
}

type MarketQuote struct {
	SecID         string
	BoardID       string
	Last          *float64
	Open          *float64
	High          *float64
	Low           *float64
	ValueToday    *float64
	VolumeToday   *int64
	UpdateTime    *string
	TradingStatus *string
	PrevClose     *float64
}

type BoardSnapshot struct {
	BoardID    string
	Securities []Security
	Quotes     map[string]MarketQuote
}

type issTable struct {
	Columns []string        `json:"columns"`
	Data    [][]interface{} `json:"data"`
}

type securitiesResponse struct {
	Securities issTable `json:"securities"`
	Marketdata issTable `json:"marketdata"`
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

func (c *Client) FetchBoard(ctx context.Context, board string) (BoardSnapshot, error) {
	market := MarketFor(board)
	isBond := market == "bonds"
	url := fmt.Sprintf("%s/engines/stock/markets/%s/boards/%s/securities.json?iss.meta=off", c.baseURL, market, board)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return BoardSnapshot{}, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return BoardSnapshot{}, fmt.Errorf("request board %s: %w", board, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return BoardSnapshot{}, fmt.Errorf("board %s: unexpected status %d", board, resp.StatusCode)
	}

	var parsed securitiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return BoardSnapshot{}, fmt.Errorf("decode board %s response: %w", board, err)
	}

	secIdx := columnIndex(parsed.Securities.Columns)
	mdIdx := columnIndex(parsed.Marketdata.Columns)

	
	
	
	prevPrice := map[string]*float64{}
	prevClose := map[string]*float64{}

	snapshot := BoardSnapshot{
		BoardID: board,
		Quotes:  make(map[string]MarketQuote, len(parsed.Marketdata.Data)),
	}

	for _, row := range parsed.Securities.Data {
		sec := Security{
			SecID:     strAt(row, secIdx, "SECID"),
			BoardID:   strAt(row, secIdx, "BOARDID"),
			ShortName: strAt(row, secIdx, "SHORTNAME"),
			SecName:   strAt(row, secIdx, "SECNAME"),
			ISIN:      strAt(row, secIdx, "ISIN"),
			Currency:  strAt(row, secIdx, "CURRENCYID"),
			Decimals:  int(intAtOr(row, secIdx, "DECIMALS", 0)),
			LotSize:   intAtOr(row, secIdx, "LOTSIZE", 0),
		}
		if sec.SecID == "" {
			continue
		}
		if p := floatPtrAt(row, secIdx, "PREVLEGALCLOSEPRICE"); p != nil && *p > 0 {
			prevClose[sec.SecID] = p
		} else if p := floatPtrAt(row, secIdx, "PREVPRICE"); p != nil && *p > 0 {
			prevClose[sec.SecID] = p
		}
		if isBond {
			sec.FaceValue = floatPtrAt(row, secIdx, "FACEVALUE")
			sec.PriceInPercent = true
			if p := floatPtrAt(row, secIdx, "PREVPRICE"); p != nil {
				prevPrice[sec.SecID] = p
			}
		}
		snapshot.Securities = append(snapshot.Securities, sec)
	}

	for _, row := range parsed.Marketdata.Data {
		secID := strAt(row, mdIdx, "SECID")
		if secID == "" {
			continue
		}
		quote := MarketQuote{
			SecID:         secID,
			BoardID:       strAt(row, mdIdx, "BOARDID"),
			Last:          floatPtrAt(row, mdIdx, "LAST"),
			Open:          floatPtrAt(row, mdIdx, "OPEN"),
			High:          floatPtrAt(row, mdIdx, "HIGH"),
			Low:           floatPtrAt(row, mdIdx, "LOW"),
			ValueToday:    floatPtrAt(row, mdIdx, "VALTODAY"),
			VolumeToday:   intPtrAt(row, mdIdx, "VOLTODAY"),
			UpdateTime:    strPtrAt(row, mdIdx, "UPDATETIME"),
			TradingStatus: strPtrAt(row, mdIdx, "TRADINGSTATUS"),
			PrevClose:     prevClose[secID],
		}
		if isBond && quote.Last == nil {
			quote.Last = floatPtrAt(row, mdIdx, "LCURRENTPRICE")
			if quote.Last == nil {
				quote.Last = prevPrice[secID]
			}
		}
		snapshot.Quotes[secID] = quote
	}

	return snapshot, nil
}

func columnIndex(columns []string) map[string]int {
	idx := make(map[string]int, len(columns))
	for i, name := range columns {
		idx[name] = i
	}
	return idx
}

func cell(row []interface{}, idx map[string]int, column string) interface{} {
	i, ok := idx[column]
	if !ok || i >= len(row) {
		return nil
	}
	return row[i]
}

func strAt(row []interface{}, idx map[string]int, column string) string {
	v := cell(row, idx, column)
	s, _ := v.(string)
	return s
}

func strPtrAt(row []interface{}, idx map[string]int, column string) *string {
	v := cell(row, idx, column)
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		return &s
	}
	return nil
}

func floatPtrAt(row []interface{}, idx map[string]int, column string) *float64 {
	v := cell(row, idx, column)
	if v == nil {
		return nil
	}
	if f, ok := v.(float64); ok {
		return &f
	}
	return nil
}

func intPtrAt(row []interface{}, idx map[string]int, column string) *int64 {
	v := cell(row, idx, column)
	if v == nil {
		return nil
	}
	if f, ok := v.(float64); ok {
		i := int64(f)
		return &i
	}
	return nil
}

func intAtOr(row []interface{}, idx map[string]int, column string, def int64) int64 {
	if p := intPtrAt(row, idx, column); p != nil {
		return *p
	}
	return def
}
