package sber

import (
	"strings"
	"errors"
	"math"
	"os"
	"testing"
	"time"

	"invest/backend/services/parser/internal/parsing"
)




func mustParse(t *testing.T, file string) parsing.Report {
	t.Helper()
	data, err := os.ReadFile("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(%s): %v", file, err)
	}
	return r
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.005 }

func TestTrades_FullPeriod(t *testing.T) {
	r := mustParse(t, "iis_2026-06-06_2026-08-05.html")
	if len(r.Trades) != 11 {
		t.Fatalf("trades = %d, want 11", len(r.Trades))
	}

	first := r.Trades[0]
	wantAt := time.Date(2026, 6, 29, 16, 37, 2, 0, moscow)
	if first.SecID != "SBER" || first.Board != "TQBR" || first.Side != "buy" ||
		first.Quantity != 12 || first.Price != 304 || first.Fee != 10.94 ||
		first.Currency != "RUB" || !first.ExecutedAt.Equal(wantAt) ||
		first.ExternalID != "sber:TEST001:trade:16947029484" {
		t.Errorf("first trade = %+v", first)
	}
}

func TestTrades_BondUsesReferenceSecidAndMoneyPrice(t *testing.T) {
	r := mustParse(t, "iis_2026-06-06_2026-08-05.html")
	var bond *parsing.Trade
	for i := range r.Trades {
		if r.Trades[i].ExternalID == "sber:TEST001:trade:17117569023" {
			bond = &r.Trades[i]
		}
	}
	if bond == nil {
		t.Fatal("OFZ trade not found")
	}
	
	
	if bond.SecID != "SU26254RMFS1" || bond.Board != "TQOB" {
		t.Errorf("bond secid/board = %s/%s, want SU26254RMFS1/TQOB", bond.SecID, bond.Board)
	}
	
	
	if bond.Price != 841 || bond.AccruedInterest != 82.26 || !near(bond.Fee, 7.83) {
		t.Errorf("bond = %+v", *bond)
	}
}

func TestCash_Classification(t *testing.T) {
	r := mustParse(t, "iis_2026-06-06_2026-08-05.html")
	count := map[string]int{}
	sum := map[string]float64{}
	for _, c := range r.CashOperations {
		count[c.Type]++
		sum[c.Type] += c.Amount
	}
	if count[parsing.CashDeposit] != 9 || !near(sum[parsing.CashDeposit], 29817.28) {
		t.Errorf("deposits: %d, %.2f; want 9, 29817.28", count[parsing.CashDeposit], sum[parsing.CashDeposit])
	}
	if count[parsing.CashDividend] != 4 || !near(sum[parsing.CashDividend], 3066.09) {
		t.Errorf("dividends: %d, %.2f; want 4, 3066.09", count[parsing.CashDividend], sum[parsing.CashDividend])
	}
	if len(r.CashOperations) != 13 {
		t.Errorf("cash ops = %d, want 13 (trade settlements and their commissions must be skipped)", len(r.CashOperations))
	}

	divs := map[string]float64{}
	for _, c := range r.CashOperations {
		if c.Type == parsing.CashDividend {
			divs[c.SecID+"/"+c.Board] = c.Amount
		}
	}
	
	want := map[string]float64{"MOEX/TQBR": 16.57, "MTSS/TQBR": 2460, "SBERP/TQBR": 130.56, "SBER/TQBR": 458.96}
	for k, v := range want {
		if !near(divs[k], v) {
			t.Errorf("dividend %s = %.2f, want %.2f", k, divs[k], v)
		}
	}
}



func TestReconcilesToClosingBalance(t *testing.T) {
	r := mustParse(t, "iis_2026-06-06_2026-08-05.html")
	balance := 0.0
	for _, c := range r.CashOperations {
		balance += c.Amount
	}
	for _, tr := range r.Trades {
		amount := tr.Quantity*tr.Price + tr.AccruedInterest
		if tr.Side == "buy" {
			balance -= amount + tr.Fee
		} else {
			balance += amount - tr.Fee
		}
	}
	if !near(balance, 2.80) {
		t.Errorf("reconciled balance = %.2f, want 2.80", balance)
	}
}

func TestOverlappingReportsShareExternalIDs(t *testing.T) {
	full := mustParse(t, "iis_2026-06-06_2026-08-05.html")
	month := mustParse(t, "iis_2026-08-01_2026-08-31.html")
	day := mustParse(t, "iis_2026-08-04_unsettled.html") 

	ids := map[string]bool{}
	for _, tr := range full.Trades {
		ids[tr.ExternalID] = true
	}
	for _, c := range full.CashOperations {
		ids[c.ExternalID] = true
	}
	// Вводные остатки (":opening:") у отчётов с разным началом периода
	// разные — какой применить, решает portfolio (см. TestOpeningBalance).
	for _, r := range []parsing.Report{month, day} {
		for _, tr := range r.Trades {
			if strings.Contains(tr.ExternalID, parsing.OpeningMarker) {
				continue
			}
			if !ids[tr.ExternalID] {
				t.Errorf("trade %s not recognised as a duplicate", tr.ExternalID)
			}
		}
		for _, c := range r.CashOperations {
			if strings.Contains(c.ExternalID, parsing.OpeningMarker) {
				continue
			}
			if !ids[c.ExternalID] {
				t.Errorf("cash op %q on %s not recognised as a duplicate", c.Description, c.Date.Format("2006-01-02"))
			}
		}
	}
}

func TestIdenticalRowsSameDayGetDistinctIDs(t *testing.T) {
	r := mustParse(t, "iis_2026-06-06_2026-08-05.html")
	seen := map[string]bool{}
	for _, c := range r.CashOperations {
		if seen[c.ExternalID] {
			t.Errorf("duplicate external id %s", c.ExternalID)
		}
		seen[c.ExternalID] = true
	}
}

func TestNoTradesReport(t *testing.T) {
	r := mustParse(t, "brokerage_2026-08-01_2026-08-31_no_trades.html")
	// Сделок нет, но на начало периода на счёте 4 бумаги — вводный остаток.
	if len(r.Trades) != 4 {
		t.Errorf("trades = %d, want 4 opening positions", len(r.Trades))
	}
	for _, tr := range r.Trades {
		if !strings.Contains(tr.ExternalID, parsing.OpeningMarker) {
			t.Errorf("unexpected non-opening trade %+v", tr)
		}
	}
	// 2 дивиденда, выплаченные на внешний счёт (пара «дивиденд +» / «вывод −»),
	// и 2 пополнения вводного остатка: бумаги и деньги.
	if len(r.CashOperations) != 6 {
		t.Fatalf("cash operations = %d, want 6: %+v", len(r.CashOperations), r.CashOperations)
	}
	var dividends, withdrawals float64
	ids := map[string]bool{}
	for _, c := range r.CashOperations {
		ids[c.ExternalID] = true
		switch {
		case strings.Contains(c.ExternalID, parsing.OpeningMarker):
		case c.Type == parsing.CashDividend:
			dividends += c.Amount
		case c.Type == parsing.CashWithdrawal:
			withdrawals += c.Amount
		default:
			t.Errorf("unexpected type %q: %+v", c.Type, c)
		}
	}
	if !near(dividends, 128.88) || !near(withdrawals, -128.88) {
		t.Errorf("dividends %.2f, withdrawals %.2f; want 128.88 / -128.88", dividends, withdrawals)
	}
	if len(ids) != 6 {
		t.Errorf("external ids not unique: %v", ids)
	}
	if first := r.CashOperations[0]; first.SecID != "SBER" || first.Board != "TQBR" {
		t.Errorf("first dividend secid/board = %s/%s, want SBER/TQBR", first.SecID, first.Board)
	}
}

func TestOpeningBalance(t *testing.T) {
	r := mustParse(t, "brokerage_2026-08-01_2026-08-31_no_trades.html")
	if r.AccountKey != "sber:TEST002" {
		t.Errorf("account key = %q, want sber:TEST002", r.AccountKey)
	}
	if r.PeriodStart == nil || r.PeriodStart.Format("2006-01-02") != "2026-08-01" {
		t.Fatalf("period start = %v, want 2026-08-01", r.PeriodStart)
	}
	want := map[string]struct {
		qty   float64
		value float64
	}{
		"SBER/TQBR": {2, 553.04},
		"T/TQBR":    {16, 4269.12},
	}
	var value, securities, cash float64
	for _, tr := range r.Trades {
		if tr.Side != "buy" || !tr.ExecutedAt.Equal(*r.PeriodStart) {
			t.Errorf("opening trade %+v: want buy at period start", tr)
		}
		value += tr.Quantity*tr.Price + tr.AccruedInterest
		if w, ok := want[tr.SecID+"/"+tr.Board]; ok && (tr.Quantity != w.qty || !near(tr.Quantity*tr.Price, w.value)) {
			t.Errorf("%s: qty %v value %.2f, want %v / %.2f", tr.SecID, tr.Quantity, tr.Quantity*tr.Price, w.qty, w.value)
		}
	}
	for _, c := range r.CashOperations {
		switch {
		case strings.HasSuffix(c.ExternalID, ":securities:RUB"):
			securities += c.Amount
		case strings.HasSuffix(c.ExternalID, ":cash:RUB"):
			cash += c.Amount
		}
	}
	// «Итого по площадке Фондовый рынок» на начало периода и остаток денег.
	if !near(value, 5119.09) || !near(securities, 5119.09) || !near(cash, 2.56) {
		t.Errorf("opening value %.2f, securities deposit %.2f, cash %.2f; want 5119.09 / 5119.09 / 2.56", value, securities, cash)
	}

	// Отчёт с нуля (счёт открыт в периоде) — вводного остатка нет.
	full := mustParse(t, "iis_2026-06-06_2026-08-05.html")
	for _, tr := range full.Trades {
		if strings.Contains(tr.ExternalID, parsing.OpeningMarker) {
			t.Errorf("unexpected opening trade in a report starting from zero: %+v", tr)
		}
	}
	for _, c := range full.CashOperations {
		if strings.Contains(c.ExternalID, parsing.OpeningMarker) {
			t.Errorf("unexpected opening cash op in a report starting from zero: %+v", c)
		}
	}
}

func TestNotSberReport(t *testing.T) {
	_, err := Parse([]byte("<html><body><table><tr><td>1</td></tr></table></body></html>"))
	if !errors.Is(err, ErrNotSberReport) {
		t.Errorf("err = %v, want ErrNotSberReport", err)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		desc   string
		amount float64
		want   string
		skip   bool
	}{
		{"Сделка от 29.06.2026", -3648, "", true},
		{"Комиссия Брокера от 29.06.2026", -10.94, "", true},
		{"Комиссия Биржи от 07.07.2026", -0.26, "", true},
		{"Зачисление д/с", 100, parsing.CashDeposit, false},
		{"Код договора S24RH4S, QR", 800, parsing.CashDeposit, false},
		{"Выплата дивидендов МТС. Налог удержан.", 2460, parsing.CashDividend, false},
		{"Выплата купонного дохода ОФЗ 26254", 112.2, parsing.CashCoupon, false},
		{"Погашение номинала ОФЗ 26254", 3000, parsing.CashRedemption, false},
		{"Списание НДФЛ", -130, parsing.CashTax, false},
		{"Вывод д/с", -1000, parsing.CashWithdrawal, false},
		{"Перевод между счетами", -500, parsing.CashWithdrawal, false},
		{"Депозитарная комиссия", -149, parsing.CashFee, false},
		{"Что-то новое", 1, parsing.CashOther, false},
	}
	for _, c := range cases {
		typ, skip := classify(c.desc, c.amount)
		if typ != c.want || skip != c.skip {
			t.Errorf("classify(%q) = %q,%v; want %q,%v", c.desc, typ, skip, c.want, c.skip)
		}
	}
}

func TestParseNumber(t *testing.T) {
	for in, want := range map[string]float64{"3 648.00": 3648, "0.1": 0.1, "+31 191.50": 31191.5, "": 0, "1 000": 1000, "-86.56": -86.56} {
		got, err := parseNumber(in)
		if err != nil || got != want {
			t.Errorf("parseNumber(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
}

func TestContainsWord(t *testing.T) {
	if containsWord("Выплата дивидендов МТСБ", "МТС") {
		t.Error("МТС must not match inside МТСБ")
	}
	if !containsWord("Выплата дивидендов МТС. Налог", "МТС") {
		t.Error("МТС must match")
	}
}
