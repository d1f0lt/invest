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

func TestCreatePortfolio_Composite(t *testing.T) {
	s := newTestServer(newFakeStore())
	ctx := withUserID("u1")
	a, _ := s.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: "ИИС"})
	b, _ := s.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: "Брокерский"})

	c, err := s.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{
		Name:      "  Всё вместе ",
		MemberIds: []string{b.Id, a.Id, b.Id, " "},
	})
	if err != nil {
		t.Fatalf("CreatePortfolio: %v", err)
	}
	if c.Name != "Всё вместе" || len(c.MemberIds) != 2 || c.MemberIds[0] != b.Id || c.MemberIds[1] != a.Id {
		t.Fatalf("got %+v", c)
	}

	got, err := s.GetPortfolio(ctx, &portfoliopb.GetPortfolioRequest{Id: c.Id})
	if err != nil || len(got.MemberIds) != 2 {
		t.Fatalf("GetPortfolio: %+v, %v", got, err)
	}

	unnamed, err := s.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{MemberIds: []string{a.Id, b.Id}})
	if err != nil || unnamed.Name != "Составной" {
		t.Fatalf("default name: %+v, %v", unnamed, err)
	}
}

func TestCreatePortfolio_CompositeValidation(t *testing.T) {
	s := newTestServer(newFakeStore())
	ctx := withUserID("u1")
	a, _ := s.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: "a"})
	b, _ := s.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: "b"})
	foreign, _ := s.CreatePortfolio(withUserID("u2"), &portfoliopb.CreatePortfolioRequest{Name: "x"})
	comp, _ := s.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: "c", MemberIds: []string{a.Id, b.Id}})

	cases := []struct {
		name    string
		members []string
		code    codes.Code
	}{
		{"one member", []string{a.Id}, codes.InvalidArgument},
		{"same member twice", []string{a.Id, a.Id}, codes.InvalidArgument},
		{"unknown member", []string{a.Id, "nope"}, codes.NotFound},
		{"someone else's member", []string{a.Id, foreign.Id}, codes.NotFound},
		{"composite member", []string{a.Id, comp.Id}, codes.InvalidArgument},
	}
	for _, tc := range cases {
		_, err := s.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: "n", MemberIds: tc.members})
		if status.Code(err) != tc.code {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.code)
		}
	}
}

func TestComposite_ReadOnly(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	ctx := withUserID("u1")
	a, _ := s.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: "a"})
	b, _ := s.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: "b"})
	c, _ := s.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: "c", MemberIds: []string{a.Id, b.Id}})

	_, err := s.CreateTrade(ctx, &portfoliopb.CreateTradeRequest{
		PortfolioId: c.Id, Secid: "SBER", Board: "TQBR", Side: "buy", Quantity: 1, Price: 1,
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("CreateTrade: %v", err)
	}
	_, err = s.CreateTrades(ctx, &portfoliopb.CreateTradesRequest{PortfolioId: c.Id, Trades: []*portfoliopb.TradeInput{
		{Secid: "SBER", Board: "TQBR", Side: "buy", Quantity: 1, Price: 1},
	}})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("CreateTrades: %v", err)
	}
	_, err = s.ImportReport(ctx, &portfoliopb.ImportReportRequest{PortfolioId: c.Id, CashOperations: []*portfoliopb.CashOperationInput{
		{Type: "deposit", Amount: 1, OccurredAt: timestamppb.Now()},
	}})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("ImportReport: %v", err)
	}
	_, err = s.CreateReportImport(ctx, &portfoliopb.CreateReportImportRequest{
		Id: "8f14e45f-ceea-4e7a-9b1d-2f7c2d9d7a10", PortfolioId: c.Id, BrokerId: "sber", Filename: "r.html",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("CreateReportImport: %v", err)
	}
	if len(store.trades[c.Id]) != 0 || len(store.cash[c.Id]) != 0 || len(store.imports) != 0 {
		t.Errorf("composite portfolio got rows of its own")
	}
}

func TestComposite_CombinesMembers(t *testing.T) {
	loc := time.FixedZone("MSK", 3*60*60)
	day := func(d int) time.Time { return time.Date(2026, 9, d, 0, 0, 0, 0, loc) }
	now := day(5).Add(14 * time.Hour)

	store := newFakeStore()
	s := newTestServer(store)
	s.Loc = loc
	s.Now = func() time.Time { return now }
	ctx := withUserID("u1")
	a, _ := s.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: "a"})
	b, _ := s.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: "b"})
	other, _ := s.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: "other"})
	c, _ := s.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{Name: "c", MemberIds: []string{a.Id, b.Id}})

	store.cash[a.Id] = []storage.CashOperation{{Type: "deposit", Amount: 3000, OccurredAt: day(1)}}
	store.cash[b.Id] = []storage.CashOperation{{Type: "deposit", Amount: 2000, OccurredAt: day(3)}}
	store.cash[other.Id] = []storage.CashOperation{{Type: "deposit", Amount: 99999, OccurredAt: day(1)}}
	for _, tr := range []*portfoliopb.CreateTradeRequest{
		{PortfolioId: a.Id, Secid: "SBER", Board: "TQBR", Side: "buy", Quantity: 10, Price: 200, ExecutedAt: timestamppb.New(day(1).Add(12 * time.Hour))},
		{PortfolioId: a.Id, Secid: "SBER", Board: "TQBR", Side: "sell", Quantity: 5, Price: 250, ExecutedAt: timestamppb.New(day(2).Add(12 * time.Hour))},
		{PortfolioId: b.Id, Secid: "SBER", Board: "TQBR", Side: "buy", Quantity: 5, Price: 260, ExecutedAt: timestamppb.New(day(3).Add(12 * time.Hour))},
		{PortfolioId: other.Id, Secid: "GAZP", Board: "TQBR", Side: "buy", Quantity: 1, Price: 100, ExecutedAt: timestamppb.New(day(1).Add(12 * time.Hour))},
	} {
		if _, err := s.CreateTrade(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}
	store.prices["SBER/TQBR"] = 300
	store.prevCloses = map[string]float64{"SBER/TQBR": 290}

	sum, err := s.GetPnL(ctx, &portfoliopb.GetPnLRequest{PortfolioId: c.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Instruments) != 1 {
		t.Fatalf("instruments = %+v", sum.Instruments)
	}
	h := sum.Instruments[0].Holding
	if h.Quantity != 10 || h.AvgCost != 230 || h.GetMarketValue() != 3000 || h.GetDayChange() != 100 {
		t.Errorf("holding = %+v", h)
	}
	if sum.TotalRealizedPnl != 250 || sum.TotalUnrealizedPnl != 700 || sum.NetDeposits != 5000 {
		t.Errorf("summary = %+v", sum)
	}
	if sum.CashBalance != 5000-2000+1250-1300 || sum.TotalDayChange != 100 {
		t.Errorf("cash/day = %v/%v", sum.CashBalance, sum.TotalDayChange)
	}

	trades, err := s.ListTrades(ctx, &portfoliopb.ListTradesRequest{PortfolioId: c.Id})
	if err != nil || len(trades.Trades) != 3 {
		t.Errorf("ListTrades: %d, %v", len(trades.GetTrades()), err)
	}
	cash, err := s.ListCashOperations(ctx, &portfoliopb.ListCashOperationsRequest{PortfolioId: c.Id})
	if err != nil || len(cash.CashOperations) != 2 {
		t.Errorf("ListCashOperations: %d, %v", len(cash.GetCashOperations()), err)
	}

	hist, err := s.GetValueHistory(ctx, &portfoliopb.GetValueHistoryRequest{PortfolioId: c.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Points) != 5 || !hist.Points[0].Date.AsTime().Equal(day(1)) {
		t.Fatalf("history: %d points", len(hist.Points))
	}
	last := hist.Points[len(hist.Points)-1]
	if last.Value != sum.CashBalance+3000 || last.NetDeposits != 5000 {
		t.Errorf("last point = %+v", last)
	}
	if hist.Points[1].NetDeposits != 3000 {
		t.Errorf("day 2 deposits = %v, b's deposit comes on day 3", hist.Points[1].NetDeposits)
	}

	_, err = s.GetPnL(withUserID("u2"), &portfoliopb.GetPnLRequest{PortfolioId: c.Id})
	if status.Code(err) != codes.NotFound {
		t.Errorf("other user: %v", err)
	}
}
