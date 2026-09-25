package dohod

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrTableNotFound = errors.New("dohod: dividend table not found")

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

type Dividend struct {
	RegistryCloseDate string
	DeclaredDate      string
	Value             float64
	Currency          string
	Forecast          bool
}

func (c *Client) Dividends(ctx context.Context, secid string) ([]Dividend, error) {
	u := c.baseURL + "/" + url.PathEscape(strings.ToLower(secid))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("dohod: build request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; invest-app)")
	req.Header.Set("Accept", "text/html")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dohod: %s: %w", secid, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dohod: %s: status %d", secid, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("dohod: %s: read: %w", secid, err)
	}
	return Parse(string(body))
}

var (
	reTable    = regexp.MustCompile(`(?is)<p[^>]*class="table-title"[^>]*>\s*Все выплаты\s*</p>\s*<table[^>]*>(.*?)</table>`)
	reRow      = regexp.MustCompile(`(?is)<tr([^>]*)>(.*?)</tr>`)
	reCell     = regexp.MustCompile(`(?is)<t([dh])[^>]*>(.*?)</t[dh]>`)
	reTag      = regexp.MustCompile(`(?s)<[^>]*>`)
	reDate     = regexp.MustCompile(`\b(\d{2})\.(\d{2})\.(\d{4})\b`)
	reNumber   = regexp.MustCompile(`^-?\d+(?:[.,]\d+)?$`)
	reForecast = regexp.MustCompile(`(?i)class="[^"]*\bforecast\b`)
)

func Parse(page string) ([]Dividend, error) {
	m := reTable.FindStringSubmatch(page)
	if m == nil {
		return nil, ErrTableNotFound
	}

	dateCol, declaredCol, valueCol := 1, 0, 3
	var out []Dividend
	headerSeen := false
	for _, row := range reRow.FindAllStringSubmatch(m[1], -1) {
		attrs, inner := row[1], row[2]
		cells := reCell.FindAllStringSubmatch(inner, -1)
		if len(cells) == 0 {
			continue
		}
		if strings.EqualFold(cells[0][1], "h") {
			headerSeen = true
			for i, c := range cells {
				t := strings.ToLower(cellText(c[2]))
				switch {
				case strings.Contains(t, "закрытия реестра"):
					dateCol = i
				case strings.Contains(t, "объявления"):
					declaredCol = i
				case t == "дивиденд" || strings.HasPrefix(t, "дивиденд"):
					valueCol = i
				}
			}
			continue
		}
		if dateCol >= len(cells) || valueCol >= len(cells) {
			continue
		}
		dateText := cellText(cells[dateCol][2])
		date := parseDate(dateText)
		if date == "" {
			continue
		}
		valueText := strings.ReplaceAll(cellText(cells[valueCol][2]), ",", ".")
		if !reNumber.MatchString(valueText) {
			continue
		}
		value, err := strconv.ParseFloat(valueText, 64)
		if err != nil {
			continue
		}
		d := Dividend{
			RegistryCloseDate: date,
			Value:             value,
			Currency:          "RUB",
			Forecast:          reForecast.MatchString(attrs) || strings.Contains(strings.ToLower(dateText), "прогноз"),
		}
		if declaredCol < len(cells) && declaredCol != dateCol {
			d.DeclaredDate = parseDate(cellText(cells[declaredCol][2]))
		}
		out = append(out, d)
	}
	if !headerSeen {
		return nil, ErrTableNotFound
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].RegistryCloseDate > out[j].RegistryCloseDate })
	return out, nil
}

func cellText(s string) string {
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}

func parseDate(s string) string {
	m := reDate.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	iso := m[3] + "-" + m[2] + "-" + m[1]
	if _, err := time.Parse("2006-01-02", iso); err != nil {
		return ""
	}
	return iso
}
