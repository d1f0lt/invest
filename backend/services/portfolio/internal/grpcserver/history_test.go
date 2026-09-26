package grpcserver

import (
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"invest/backend/services/portfolio/internal/storage"
	portfoliopb "invest/backend/services/portfolio/proto"
)

func TestGetPnL_DayChange(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	p, _ := s.CreatePortfolio(withUserID("u1"), &portfoliopb.CreatePortfolioRequest{})
	if _, err := s.CreateTrade(withUserID("u1"), &portfoliopb.CreateTradeRequest{
		PortfolioId: p.Id, Secid: "SBER", Board: "TQBR", Side: "buy", Quantity: 10, Price: 100,
	}); err != nil {
		t.Fatal(err)
	}
	store.prices["SBER/TQBR"] = 120
	store.prevCloses = map[string]float64{"SBER/TQBR": 125}

	summary, err := s.GetPnL(withUserID("u1"), &portfoliopb.GetPnLRequest{PortfolioId: p.Id})
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalDayChange != -50 {
		t.Errorf("total_day_change = %v, want -50", summary.TotalDayChange)
	}
	if got := summary.Instruments[0].GetHolding().GetDayChange(); got != -50 {
		t.Errorf("day_change = %v, want -50", got)
	}
}

func TestGetValueHistory(t *testing.T) {
	loc := time.FixedZone("MSK", 3*60*60)
	day := func(d int) time.Time { return time.Date(2026, 9, d, 0, 0, 0, 0, loc) }
	now := day(10).Add(14 * time.Hour)

	store := newFakeStore()
	s := newTestServer(store)
	s.Loc = loc
	s.Now = func() time.Time { return now }
	p, _ := s.CreatePortfolio(withUserID("u1"), &portfoliopb.CreatePortfolioRequest{})

	empty, err := s.GetValueHistory(withUserID("u1"), &portfoliopb.GetValueHistoryRequest{PortfolioId: p.Id})
	if err != nil || len(empty.Points) != 0 {
		t.Fatalf("empty portfolio: %v, %v", empty, err)
	}

	store.cash[p.Id] = []storage.CashOperation{{Type: "deposit", Amount: 1000, OccurredAt: day(1).Add(10 * time.Hour)}}
	if _, err := s.CreateTrade(withUserID("u1"), &portfoliopb.CreateTradeRequest{
		PortfolioId: p.Id, Secid: "SBER", Board: "TQBR", Side: "buy", Quantity: 5, Price: 100,
		ExecutedAt: timestamppb.New(day(2).Add(12 * time.Hour)),
	}); err != nil {
		t.Fatal(err)
	}
	store.closes = map[string][]storage.DailyClose{"SBER/TQBR": {{Day: day(2), Close: 110}, {Day: day(9), Close: 90}}}
	store.prices["SBER/TQBR"] = 95

	all, err := s.GetValueHistory(withUserID("u1"), &portfoliopb.GetValueHistoryRequest{PortfolioId: p.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Points) != 10 {
		t.Fatalf("all: %d points, want 10 (from the first deposit)", len(all.Points))
	}
	first, last := all.Points[0], all.Points[len(all.Points)-1]
	if !first.Date.AsTime().Equal(day(1)) || first.Value != 1000 || first.NetDeposits != 1000 {
		t.Errorf("first point = %+v", first)
	}
	if all.Points[1].Value != 500+5*110 || all.Points[8].Value != 500+5*90 {
		t.Errorf("closes: day2 %v, day9 %v", all.Points[1].Value, all.Points[8].Value)
	}
	if last.Value != 500+5*95 {
		t.Errorf("last point = %v, want current prices", last.Value)
	}

	week, err := s.GetValueHistory(withUserID("u1"), &portfoliopb.GetValueHistoryRequest{PortfolioId: p.Id, Range: "week"})
	if err != nil {
		t.Fatal(err)
	}
	if len(week.Points) != 8 || !week.Points[0].Date.AsTime().Equal(day(3)) {
		t.Errorf("week: %d points from %v", len(week.Points), week.Points[0].Date.AsTime())
	}
	if week.Points[0].Value != 500+5*110 {
		t.Errorf("week first = %v: close before the period must be used", week.Points[0].Value)
	}
	if !store.closesFrom.Before(day(3)) {
		t.Errorf("closes loaded from %v, need a lookback", store.closesFrom)
	}

	_, err = s.GetValueHistory(withUserID("u1"), &portfoliopb.GetValueHistoryRequest{PortfolioId: p.Id, Range: "decade"})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("bad range: %v", err)
	}
	_, err = s.GetValueHistory(withUserID("u2"), &portfoliopb.GetValueHistoryRequest{PortfolioId: p.Id})
	if status.Code(err) != codes.NotFound {
		t.Errorf("other user: %v", err)
	}
}
