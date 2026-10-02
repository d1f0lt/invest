package grpcserver

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"invest/backend/services/portfolio/internal/storage"
	portfoliopb "invest/backend/services/portfolio/proto"
)

func TestPickPriceBoard(t *testing.T) {
	now := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		board string
		seen  []storage.BoardPrice
		want  string
	}{
		{"no prices", "TQTF", nil, "TQTF"},
		{"own board only", "TQBR", []storage.BoardPrice{{Board: "TQBR", CollectedAt: now}}, "TQBR"},
		{"moved board", "TQTF", []storage.BoardPrice{{Board: "TQBR", CollectedAt: now}}, "TQBR"},
		{"stale own board", "TQTF", []storage.BoardPrice{
			{Board: "TQTF", CollectedAt: now.AddDate(0, -3, 0)},
			{Board: "TQBR", CollectedAt: now},
		}, "TQBR"},
		{"both fresh", "TQTF", []storage.BoardPrice{
			{Board: "TQTF", CollectedAt: now.Add(-time.Hour)},
			{Board: "TQBR", CollectedAt: now},
		}, "TQTF"},
	}
	for _, c := range cases {
		if got := pickPriceBoard(c.board, c.seen); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestGetValueHistory_MovedBoard(t *testing.T) {
	loc := time.FixedZone("MSK", 3*60*60)
	day := func(d int) time.Time { return time.Date(2026, 9, d, 0, 0, 0, 0, loc) }
	now := day(6).Add(14 * time.Hour)

	store := newFakeStore()
	s := newTestServer(store)
	s.Loc = loc
	s.Now = func() time.Time { return now }
	p, _ := s.CreatePortfolio(withUserID("u1"), &portfoliopb.CreatePortfolioRequest{})
	if _, err := s.CreateTrade(withUserID("u1"), &portfoliopb.CreateTradeRequest{
		PortfolioId: p.Id, Secid: "SBMX", Board: "TQTF", Side: "buy", Quantity: 10, Price: 15,
		ExecutedAt: timestamppb.New(day(1).Add(12 * time.Hour)),
	}); err != nil {
		t.Fatal(err)
	}
	store.boards = map[string][]storage.BoardPrice{"SBMX": {{Board: "TQBR", CollectedAt: now}}}
	store.prices["SBMX/TQBR"] = 17
	store.prevCloses = map[string]float64{"SBMX/TQBR": 18}
	store.closes = map[string][]storage.DailyClose{
		"SBMX/TQTF": {{Day: day(1), Close: 15.5}, {Day: day(2), Close: 99}},
		"SBMX/TQBR": {{Day: day(2), Close: 16}, {Day: day(4), Close: 16.5}},
	}

	hist, err := s.GetValueHistory(withUserID("u1"), &portfoliopb.GetValueHistoryRequest{PortfolioId: p.Id})
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{155, 160, 160, 165, 165, 170}
	if len(hist.Points) != len(want) {
		t.Fatalf("points = %d, want %d", len(hist.Points), len(want))
	}
	for i, w := range want {
		if got := hist.Points[i].Value - hist.Points[i].NetDeposits + 150; got != w {
			t.Errorf("day %d: value = %v, want %v", i+1, got, w)
		}
	}

	summary, err := s.GetPnL(withUserID("u1"), &portfoliopb.GetPnLRequest{PortfolioId: p.Id})
	if err != nil {
		t.Fatal(err)
	}
	if got := summary.Instruments[0].GetHolding().GetDayChange(); got != -10 {
		t.Errorf("day_change = %v, want -10", got)
	}
	if got := summary.Instruments[0].GetHolding().GetBoard(); got != "TQBR" {
		t.Errorf("board = %s, want TQBR", got)
	}
}
