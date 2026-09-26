

















package sber

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/text/encoding/charmap"

	"invest/backend/services/parser/internal/parsing"
	"invest/backend/services/parser/internal/task"
)



const BrokerKey = "sber"



var ErrNotSberReport = errors.New("sber: file is not a Sberbank HTML broker report")



var moscow = time.FixedZone("MSK", 3*60*60)


type Parser struct{}

func (Parser) Parse(_ task.ReportUploaded, data []byte) (parsing.Report, error) {
	return Parse(data)
}


var (
	headTrades    = normKey("Сделки купли/продажи ценных бумаг")
	headCash      = normKey("Движение денежных средств за период")
	headSecurites = normKey("Справочник Ценных Бумаг")
	headPayouts   = normKey("Выплаты дохода от эмитента на внешний счет")
	headTitle     = normKey("Отчет брокера")
	headHoldings  = normKey("Портфель Ценных Бумаг")
	headCashBal   = normKey("Денежные средства")
)


func Parse(data []byte) (parsing.Report, error) {
	doc, err := html.Parse(bytes.NewReader(toUTF8(data)))
	if err != nil {
		return parsing.Report{}, fmt.Errorf("sber: parse html: %w", err)
	}
	d := collect(doc)
	if !d.hasTitle {
		return parsing.Report{}, ErrNotSberReport
	}

	ref, err := parseReference(d.sections[headSecurites])
	if err != nil {
		return parsing.Report{}, err
	}

	trades, err := parseTrades(d.sections[headTrades], ref, d.account)
	if err != nil {
		return parsing.Report{}, err
	}
	cash, err := parseCash(d.sections[headCash], ref, d.account)
	if err != nil {
		return parsing.Report{}, err
	}
	payouts, err := parseExternalPayouts(d.sections[headPayouts], ref, d.account)
	if err != nil {
		return parsing.Report{}, err
	}
	cash = append(cash, payouts...)

	report := parsing.Report{Trades: trades, CashOperations: cash}
	if d.account != "" {
		report.AccountKey = "sber:" + d.account
	}
	if d.periodStart != nil && report.AccountKey != "" {
		report.PeriodStart = d.periodStart
		var holdings [][]row
		for key, tables := range d.sections {
			if strings.HasPrefix(key, headHoldings) {
				holdings = append(holdings, tables...)
			}
		}
		oTrades, oCash, err := parseOpening(holdings, d.sections[headCashBal], ref, report.AccountKey, *d.periodStart)
		if err != nil {
			return parsing.Report{}, err
		}
		report.Trades = append(report.Trades, oTrades...)
		report.CashOperations = append(report.CashOperations, oCash...)
	}
	return report, nil
}




type row struct {
	cells  []string
	header bool 
}

type document struct {
	hasTitle bool
	account  string 
	// Начало периода из заголовка «Отчет брокера за период с … по …».
	periodStart *time.Time
	
	
	sections map[string][][]row
}

var reAccountInTitle = regexp.MustCompile(`(?i)отчет брокера\s+(\S+)`)

var rePeriod = regexp.MustCompile(`(?i)за период с\s+(\d{2}\.\d{2}\.\d{4})\s+по\s+(\d{2}\.\d{2}\.\d{4})`)

func collect(root *html.Node) document {
	d := document{sections: map[string][][]row{}}
	var current string

	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				t := collapse(textOf(n))
				if strings.Contains(normKey(t), headTitle) {
					d.hasTitle = true
					if m := reAccountInTitle.FindStringSubmatch(t); m != nil {
						d.account = m[1]
					}
				}
				return
			case "h1", "h2", "h3":
				if m := rePeriod.FindStringSubmatch(collapse(textOf(n))); m != nil && d.periodStart == nil {
					if t, err := parseDate(m[1]); err == nil {
						d.periodStart = &t
					}
				}
				if strings.HasPrefix(normKey(textOf(n)), headTitle) {
					d.hasTitle = true
				}
				return
			case "p":
				if h := normKey(textOf(n)); h != "" {
					current = h
					if strings.HasPrefix(h, headTitle) {
						d.hasTitle = true
					}
				}
				return
			case "table":
				d.sections[current] = append(d.sections[current], readTable(n))
				return 
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return d
}

func readTable(t *html.Node) []row {
	var rows []row
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			r := row{header: hasClass(n, "table-header")}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
					r.cells = append(r.cells, collapse(textOf(c)))
				}
			}
			rows = append(rows, r)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(t)
	return rows
}

func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		switch {
		case n.Type == html.TextNode:
			b.WriteString(n.Data)
		case n.Type == html.ElementNode && n.Data == "br":
			b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func hasClass(n *html.Node, class string) bool {
	for _, a := range n.Attr {
		if a.Key == "class" {
			for _, c := range strings.Fields(a.Val) {
				if c == class {
					return true
				}
			}
		}
	}
	return false
}




type columns map[string]int

func headerOf(rows []row) (columns, bool) {
	for _, r := range rows {
		if r.header {
			cols := columns{}
			for i, c := range r.cells {
				cols[normKey(c)] = i
			}
			return cols, true
		}
	}
	return nil, false
}

func (c columns) require(names ...string) error {
	var missing []string
	for _, n := range names {
		if _, ok := c[normKey(n)]; !ok {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("sber: table layout changed, missing columns %q", missing)
	}
	return nil
}

func (c columns) get(r row, name string) string {
	i, ok := c[normKey(name)]
	if !ok || i >= len(r.cells) {
		return ""
	}
	return r.cells[i]
}




func dataRows(rows []row, width int) []row {
	var out []row
	for _, r := range rows {
		if r.header || len(r.cells) != width {
			continue
		}
		if _, err := parseDate(r.cells[0]); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out
}




type security struct {
	Name  string
	Code  string 
	ISIN  string
	Kind  string 
	Board string
}

type reference struct {
	byCode map[string]*security 
	all    []*security
}

func parseReference(tables [][]row) (*reference, error) {
	ref := &reference{byCode: map[string]*security{}}
	for _, rows := range tables {
		cols, ok := headerOf(rows)
		if !ok {
			continue
		}
		if err := cols.require("Наименование", "Код", "ISIN ценной бумаги", "Вид, Категория, Тип, иная информация"); err != nil {
			return nil, err
		}
		for _, r := range rows {
			if r.header || len(r.cells) != len(cols) {
				continue
			}
			s := &security{
				Name: cols.get(r, "Наименование"),
				Code: strings.ToUpper(cols.get(r, "Код")),
				ISIN: strings.ToUpper(cols.get(r, "ISIN ценной бумаги")),
				Kind: cols.get(r, "Вид, Категория, Тип, иная информация"),
			}
			if s.Code == "" || s.Name == "" || isColumnNumber(s.Name) {
				continue
			}
			s.Board = boardFor(s.Kind)
			ref.all = append(ref.all, s)
			ref.byCode[s.Code] = s
			if s.ISIN != "" {
				ref.byCode[s.ISIN] = s
			}
		}
	}
	return ref, nil
}





func boardFor(kind string) string {
	k := strings.ToLower(kind)
	switch {
	case strings.Contains(k, "акци"), strings.Contains(k, "депозитарн"):
		return "TQBR"
	case strings.Contains(k, "пай"), strings.Contains(k, "фонд"), strings.Contains(k, "etf"):
		return "TQTF"
	case strings.Contains(k, "государственн") && strings.Contains(k, "облигац"):
		return "TQOB"
	case strings.Contains(k, "облигац"):
		return "TQCB"
	}
	return ""
}

func (r *reference) lookup(code, name string) (*security, error) {
	if s, ok := r.byCode[strings.ToUpper(code)]; ok {
		if s.Board == "" {
			return nil, fmt.Errorf("sber: unsupported instrument kind %q for %s (%s)", s.Kind, s.Name, s.Code)
		}
		return s, nil
	}
	if s := r.findInText(name); s != nil && s.Name == name {
		if s.Board == "" {
			return nil, fmt.Errorf("sber: unsupported instrument kind %q for %s (%s)", s.Kind, s.Name, s.Code)
		}
		return s, nil
	}
	return nil, fmt.Errorf("sber: security %q (%s) not found in the report's securities reference", name, code)
}




func (r *reference) findInText(s string) *security {
	var best *security
	for _, sec := range r.all {
		if containsWord(s, sec.Name) && (best == nil || len(sec.Name) > len(best.Name)) {
			best = sec
		}
	}
	return best
}



func containsWord(s, name string) bool {
	for from := 0; ; {
		i := strings.Index(s[from:], name)
		if i < 0 {
			return false
		}
		end := from + i + len(name)
		if end == len(s) {
			return true
		}
		next, _ := utf8.DecodeRuneInString(s[end:])
		if !unicode.IsLetter(next) && !unicode.IsDigit(next) {
			return true
		}
		from = from + i + 1
	}
}




func parseTrades(tables [][]row, ref *reference, account string) ([]parsing.Trade, error) {
	var out []parsing.Trade
	for _, rows := range tables {
		cols, ok := headerOf(rows)
		if !ok {
			continue
		}
		if err := cols.require(
			"Дата заключения", "Время заключения", "Наименование ЦБ", "Код ЦБ", "Валюта", "Вид",
			"Количество, шт.", "Цена", "Сумма", "НКД", "Комиссия Брокера", "Комиссия Биржи", "Номер сделки",
		); err != nil {
			return nil, err
		}
		for _, r := range dataRows(rows, len(cols)) {
			t, err := parseTradeRow(cols, r, ref, account)
			if err != nil {
				return nil, err
			}
			out = append(out, t)
		}
	}
	return out, nil
}

func parseTradeRow(cols columns, r row, ref *reference, account string) (parsing.Trade, error) {
	name := cols.get(r, "Наименование ЦБ")
	code := cols.get(r, "Код ЦБ")
	dealNo := cols.get(r, "Номер сделки")
	fail := func(format string, args ...any) error {
		return fmt.Errorf("sber: trade %s (%s): %s", dealNo, name, fmt.Sprintf(format, args...))
	}

	var side string
	switch strings.ToLower(cols.get(r, "Вид")) {
	case "покупка":
		side = "buy"
	case "продажа":
		side = "sell"
	default:
		return parsing.Trade{}, fail("unsupported trade kind %q", cols.get(r, "Вид"))
	}

	sec, err := ref.lookup(code, name)
	if err != nil {
		return parsing.Trade{}, err
	}

	executedAt, err := parseDateTime(cols.get(r, "Дата заключения"), cols.get(r, "Время заключения"))
	if err != nil {
		return parsing.Trade{}, fail("%v", err)
	}

	nums := map[string]float64{}
	for _, c := range []string{"Количество, шт.", "Сумма", "НКД", "Комиссия Брокера", "Комиссия Биржи"} {
		v, err := parseNumber(cols.get(r, c))
		if err != nil {
			return parsing.Trade{}, fail("column %q: %v", c, err)
		}
		nums[c] = v
	}
	qty := nums["Количество, шт."]
	if qty <= 0 {
		return parsing.Trade{}, fail("non-positive quantity %v", qty)
	}
	if dealNo == "" {
		return parsing.Trade{}, fail("empty deal number")
	}

	return parsing.Trade{
		SecID:    sec.Code,
		Board:    sec.Board,
		Side:     side,
		Quantity: qty,
		
		
		
		Price:           round(nums["Сумма"]/qty, 6),
		Fee:             round(nums["Комиссия Брокера"]+nums["Комиссия Биржи"], 2),
		AccruedInterest: nums["НКД"],
		Currency:        strings.ToUpper(cols.get(r, "Валюта")),
		ExecutedAt:      &executedAt,
		ExternalID:      "sber:" + account + ":trade:" + dealNo,
		SecurityName:    sec.Name,
		ISIN:            sec.ISIN,
	}, nil
}




func parseCash(tables [][]row, ref *reference, account string) ([]parsing.CashOperation, error) {
	var out []parsing.CashOperation
	seen := map[string]int{}
	for _, rows := range tables {
		cols, ok := headerOf(rows)
		if !ok {
			continue
		}
		if err := cols.require("Дата", "Описание операции", "Валюта", "Сумма зачисления", "Сумма списания"); err != nil {
			return nil, err
		}
		for _, r := range dataRows(rows, len(cols)) {
			desc := cols.get(r, "Описание операции")
			date, err := parseDate(cols.get(r, "Дата"))
			if err != nil {
				return nil, fmt.Errorf("sber: cash row %q: %w", desc, err)
			}
			in, err := parseNumber(cols.get(r, "Сумма зачисления"))
			if err != nil {
				return nil, fmt.Errorf("sber: cash row %q: %w", desc, err)
			}
			outAmt, err := parseNumber(cols.get(r, "Сумма списания"))
			if err != nil {
				return nil, fmt.Errorf("sber: cash row %q: %w", desc, err)
			}
			amount := round(in-outAmt, 2)
			if amount == 0 {
				continue
			}

			typ, skip := classify(desc, amount)
			if skip {
				continue
			}
			op := parsing.CashOperation{
				Type:        typ,
				Amount:      amount,
				Currency:    strings.ToUpper(cols.get(r, "Валюта")),
				Date:        date,
				Description: desc,
			}
			if typ == parsing.CashDividend || typ == parsing.CashCoupon || typ == parsing.CashRedemption {
				if sec := ref.findInText(desc); sec != nil {
					op.SecID, op.Board = sec.Code, sec.Board
				}
			}

			
			
			
			
			
			
			key := strings.Join([]string{account, date.Format("2006-01-02"), op.Currency, desc, strconv.FormatFloat(amount, 'f', 2, 64)}, "|")
			n := seen[key]
			seen[key]++
			sum := sha1.Sum([]byte(fmt.Sprintf("%s#%d", key, n)))
			op.ExternalID = "sber:" + account + ":cash:" + hex.EncodeToString(sum[:12])

			out = append(out, op)
		}
	}
	return out, nil
}





// parseExternalPayouts разбирает «Выплаты дохода от эмитента на внешний счет»:
// дивиденды/купоны, которые эмитент перечислил сразу на банковский счёт, минуя
// брокерский. Это доход портфеля, но на брокерский счёт деньги не приходили,
// поэтому каждая выплата даёт пару операций: доход (+) и вывод (−) на ту же
// сумму. Так P&L видит доход, а остаток денег на счёте не меняется.
func parseExternalPayouts(tables [][]row, ref *reference, account string) ([]parsing.CashOperation, error) {
	var out []parsing.CashOperation
	seen := map[string]int{}
	for _, rows := range tables {
		cols, ok := headerOf(rows)
		if !ok {
			continue
		}
		if err := cols.require("Дата", "Описание операции", "Валюта", "Сумма"); err != nil {
			return nil, err
		}
		for _, r := range dataRows(rows, len(cols)) {
			desc := cols.get(r, "Описание операции")
			date, err := parseDate(cols.get(r, "Дата"))
			if err != nil {
				return nil, fmt.Errorf("sber: external payout %q: %w", desc, err)
			}
			amount, err := parseNumber(cols.get(r, "Сумма"))
			if err != nil {
				return nil, fmt.Errorf("sber: external payout %q: %w", desc, err)
			}
			amount = round(amount, 2)
			if amount <= 0 {
				continue
			}
			typ, _ := classify(desc, amount)
			switch typ {
			case parsing.CashDividend, parsing.CashCoupon, parsing.CashRedemption:
			default:
				typ = parsing.CashOther
			}
			currency := strings.ToUpper(cols.get(r, "Валюта"))

			key := strings.Join([]string{account, date.Format("2006-01-02"), currency, desc, strconv.FormatFloat(amount, 'f', 2, 64)}, "|")
			n := seen[key]
			seen[key]++
			id := func(tag string) string {
				sum := sha1.Sum([]byte(fmt.Sprintf("%s|%s#%d", tag, key, n)))
				return "sber:" + account + ":" + tag + ":" + hex.EncodeToString(sum[:12])
			}

			income := parsing.CashOperation{
				Type:        typ,
				Amount:      amount,
				Currency:    currency,
				Date:        date,
				Description: desc + " (на внешний счёт)",
				ExternalID:  id("payout"),
			}
			if sec := ref.findInText(desc); sec != nil {
				income.SecID, income.Board = sec.Code, sec.Board
			}
			out = append(out, income, parsing.CashOperation{
				Type:        parsing.CashWithdrawal,
				Amount:      -amount,
				Currency:    currency,
				Date:        date,
				Description: "Выплата на внешний счёт: " + desc,
				ExternalID:  id("payout-out"),
			})
		}
	}
	return out, nil
}

// parseOpening превращает «Портфель Ценных Бумаг» (колонки «Начало периода»)
// и «Денежные средства» (остаток на начало) во вводный остаток:
//   - каждая бумага → покупка по рыночной стоимости на начало периода
//     (себестоимости в отчёте нет), НКД — как при обычной покупке;
//   - стоимость всех бумаг → одно пополнение ":securities" (деньги, которыми
//     эти бумаги «оплачены», чтобы остаток денег не ушёл в минус);
//   - деньги на начало периода → пополнение ":cash:<валюта>".
//
// Время — начало периода (00:00 МСК). Если на начало периода счёт пуст,
// ничего не возвращает.
func parseOpening(holdings, cashTables [][]row, ref *reference, accountKey string, start time.Time) ([]parsing.Trade, []parsing.CashOperation, error) {
	prefix := accountKey + parsing.OpeningMarker + start.Format("2006-01-02") + ":"
	dateText := start.Format("02.01.2006")

	var trades []parsing.Trade
	costs := map[string]float64{}
	for _, rows := range holdings {
		hdr, ok := namedHeader(rows, "Наименование")
		if !ok {
			continue
		}
		idx := firstIndexes(hdr.cells)
		for _, name := range []string{"Наименование", "ISIN ценной бумаги", "Валюта рыночной цены", "Количество, шт", "Рыночная стоимость, без НКД", "НКД"} {
			if _, ok := idx[normKey(name)]; !ok {
				return nil, nil, fmt.Errorf("sber: securities portfolio table layout changed, missing column %q", name)
			}
		}
		get := func(r row, name string) string {
			i := idx[normKey(name)]
			if i >= len(r.cells) {
				return ""
			}
			return r.cells[i]
		}
		for _, r := range rows {
			if r.header || len(r.cells) != len(hdr.cells) {
				continue
			}
			name, isin := get(r, "Наименование"), strings.ToUpper(get(r, "ISIN ценной бумаги"))
			if name == "" || isin == "" || isColumnNumber(name) {
				continue
			}
			qty, err := parseNumber(get(r, "Количество, шт"))
			if err != nil {
				return nil, nil, fmt.Errorf("sber: opening position %s: %w", name, err)
			}
			if qty <= 0 {
				continue
			}
			value, err := parseNumber(get(r, "Рыночная стоимость, без НКД"))
			if err != nil {
				return nil, nil, fmt.Errorf("sber: opening position %s: %w", name, err)
			}
			accrued, err := parseNumber(get(r, "НКД"))
			if err != nil {
				return nil, nil, fmt.Errorf("sber: opening position %s: %w", name, err)
			}
			sec, err := ref.lookup(isin, name)
			if err != nil {
				return nil, nil, err
			}
			currency := strings.ToUpper(get(r, "Валюта рыночной цены"))
			if currency == "" {
				currency = "RUB"
			}
			at := start
			trades = append(trades, parsing.Trade{
				SecID:           sec.Code,
				Board:           sec.Board,
				Side:            "buy",
				Quantity:        qty,
				Price:           round(value/qty, 6),
				AccruedInterest: round(accrued, 2),
				Currency:        currency,
				ExecutedAt:      &at,
				ExternalID:      prefix + "position:" + isin,
				SecurityName:    sec.Name,
				ISIN:            sec.ISIN,
			})
			costs[currency] += value + accrued
		}
	}

	var cash []parsing.CashOperation
	for _, currency := range sortedKeys(costs) {
		amount := round(costs[currency], 2)
		if amount <= 0 {
			continue
		}
		cash = append(cash, parsing.CashOperation{
			Type:        parsing.CashDeposit,
			Amount:      amount,
			Currency:    currency,
			Date:        start,
			Description: "Вводный остаток: бумаги на " + dateText,
			ExternalID:  prefix + "securities:" + currency,
		})
	}

	balances := map[string]float64{}
	for _, rows := range cashTables {
		hdr, ok := namedHeader(rows, "Торговая площадка")
		if !ok {
			continue
		}
		idx := firstIndexes(hdr.cells)
		ci, okC := idx[normKey("Валюта")]
		si, okS := idx[normKey("Начало периода")]
		if !okC || !okS {
			return nil, nil, fmt.Errorf("sber: cash balance table layout changed")
		}
		for _, r := range rows {
			if r.header || len(r.cells) != len(hdr.cells) || !strings.HasPrefix(strings.ToLower(r.cells[0]), "торговый счет") {
				continue
			}
			v, err := parseNumber(r.cells[si])
			if err != nil {
				return nil, nil, fmt.Errorf("sber: opening cash: %w", err)
			}
			balances[strings.ToUpper(r.cells[ci])] += v
		}
	}
	for _, currency := range sortedKeys(balances) {
		amount := round(balances[currency], 2)
		if amount == 0 {
			continue
		}
		typ := parsing.CashDeposit
		if amount < 0 {
			typ = parsing.CashWithdrawal
		}
		cash = append(cash, parsing.CashOperation{
			Type:        typ,
			Amount:      amount,
			Currency:    currency,
			Date:        start,
			Description: "Вводный остаток: деньги на " + dateText,
			ExternalID:  prefix + "cash:" + currency,
		})
	}
	return trades, cash, nil
}

// namedHeader — строка-заголовок таблицы, где первая ячейка = first
// (у «Портфеля Ценных Бумаг» над ней есть ещё строка групп колонок).
func namedHeader(rows []row, first string) (row, bool) {
	for _, r := range rows {
		if r.header && len(r.cells) > 0 && normKey(r.cells[0]) == normKey(first) {
			return r, true
		}
	}
	return row{}, false
}

// firstIndexes — индекс первой колонки с таким названием (названия
// повторяются: «Количество, шт» есть у начала и у конца периода).
func firstIndexes(cells []string) map[string]int {
	idx := map[string]int{}
	for i, c := range cells {
		k := normKey(c)
		if _, ok := idx[k]; !ok {
			idx[k] = i
		}
	}
	return idx
}

func sortedKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func classify(desc string, amount float64) (typ string, skip bool) {
	d := strings.ToLower(desc)
	switch {
	case strings.HasPrefix(d, "сделка от"),
		strings.HasPrefix(d, "комиссия брокера от"),
		strings.HasPrefix(d, "комиссия биржи от"):
		return "", true
	case strings.Contains(d, "дивиденд"):
		return parsing.CashDividend, false
	case strings.Contains(d, "купон"):
		return parsing.CashCoupon, false
	case strings.Contains(d, "погашени"), strings.Contains(d, "амортизаци"):
		return parsing.CashRedemption, false
	case strings.Contains(d, "налог"), strings.Contains(d, "ндфл"):
		return parsing.CashTax, false
	case strings.Contains(d, "комисси"):
		return parsing.CashFee, false
	case strings.Contains(d, "вывод"), strings.Contains(d, "списание д/с"):
		return parsing.CashWithdrawal, false
	case strings.Contains(d, "зачисление д/с"),
		strings.Contains(d, "пополнение"),
		strings.Contains(d, ", qr"),
		strings.Contains(d, "перевод"):
		
		
		if amount > 0 {
			return parsing.CashDeposit, false
		}
		return parsing.CashWithdrawal, false
	}
	return parsing.CashOther, false
}




func parseDate(s string) (time.Time, error) {
	t, err := time.ParseInLocation("02.01.2006", strings.TrimSpace(s), moscow)
	if err != nil {
		return time.Time{}, fmt.Errorf("bad date %q", s)
	}
	return t, nil
}

func parseDateTime(date, clock string) (time.Time, error) {
	t, err := time.ParseInLocation("02.01.2006 15:04:05", strings.TrimSpace(date)+" "+strings.TrimSpace(clock), moscow)
	if err != nil {
		return time.Time{}, fmt.Errorf("bad date/time %q %q", date, clock)
	}
	return t, nil
}

var numberCleaner = strings.NewReplacer(" ", "", " ", "", " ", "", ",", ".")



func parseNumber(s string) (float64, error) {
	s = numberCleaner.Replace(strings.TrimSpace(s))
	if s == "" || s == "-" {
		return 0, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("bad number %q", s)
	}
	return v, nil
}

func round(v float64, digits int) float64 {
	p := 1.0
	for i := 0; i < digits; i++ {
		p *= 10
	}
	r, _ := strconv.ParseFloat(strconv.FormatFloat(v*p, 'f', 0, 64), 64)
	return r / p
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }



func normKey(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r):
			if r == 'ё' {
				r = 'е'
			}
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		default:
			space = true
		}
	}
	return b.String()
}

func isColumnNumber(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}



func toUTF8(data []byte) []byte {
	if utf8.Valid(data) {
		return data
	}
	out, err := charmap.Windows1251.NewDecoder().Bytes(data)
	if err != nil {
		return data
	}
	return out
}
