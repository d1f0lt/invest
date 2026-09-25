package grpcserver

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"invest/backend/services/portfolio/internal/authmd"
	"invest/backend/services/portfolio/internal/storage"
	portfoliopb "invest/backend/services/portfolio/proto"
)

type fakeStore struct {
	portfolios map[string]storage.Portfolio
	trades     map[string][]storage.Trade
	cash       map[string][]storage.CashOperation
	prices     map[string]float64
	nextID     int

	brokers map[string]storage.Broker
	imports map[string]storage.ReportImport
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		portfolios: map[string]storage.Portfolio{},
		trades:     map[string][]storage.Trade{},
		cash:       map[string][]storage.CashOperation{},
		prices:     map[string]float64{},
		brokers: map[string]storage.Broker{
			"sber": {ID: "sber", Name: "СберИнвестиции", FileFormats: []string{"html"}, Enabled: true},
			"old":  {ID: "old", Name: "Old", FileFormats: []string{"html"}, Enabled: false},
		},
		imports: map[string]storage.ReportImport{},
	}
}

func (f *fakeStore) genID(prefix string) string {
	f.nextID++
	return prefix + "-" + time.Now().Format("150405") + "-" + string(rune('a'+f.nextID))
}

func (f *fakeStore) CreatePortfolio(_ context.Context, userID, name string) (storage.Portfolio, error) {
	p := storage.Portfolio{ID: f.genID("p"), UserID: userID, Name: name, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	f.portfolios[p.ID] = p
	return p, nil
}

func (f *fakeStore) ListPortfoliosByUser(_ context.Context, userID string) ([]storage.Portfolio, error) {
	var out []storage.Portfolio
	for _, p := range f.portfolios {
		if p.UserID == userID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeStore) GetPortfolio(_ context.Context, id string) (storage.Portfolio, error) {
	if p, ok := f.portfolios[id]; ok {
		return p, nil
	}
	return storage.Portfolio{}, storage.ErrNotFound
}

func (f *fakeStore) RenamePortfolio(_ context.Context, id, name string) (storage.Portfolio, error) {
	p, ok := f.portfolios[id]
	if !ok {
		return storage.Portfolio{}, storage.ErrNotFound
	}
	p.Name = name
	p.UpdatedAt = time.Now()
	f.portfolios[id] = p
	return p, nil
}

func (f *fakeStore) CreateTrade(_ context.Context, t storage.Trade) (storage.Trade, error) {
	if t.SecID == "UNKNOWN" {
		return storage.Trade{}, storage.ErrUnknownInstrument
	}
	t.ID = f.genID("t")
	f.trades[t.PortfolioID] = append(f.trades[t.PortfolioID], t)
	return t, nil
}

func (f *fakeStore) CreateTradesBatch(_ context.Context, trades []storage.Trade) ([]storage.Trade, error) {
	
	
	created := make([]storage.Trade, 0, len(trades))
	for _, t := range trades {
		if t.SecID == "UNKNOWN" {
			return nil, storage.ErrUnknownInstrument
		}
		c := t
		c.ID = f.genID("t")
		created = append(created, c)
	}
	for _, c := range created {
		f.trades[c.PortfolioID] = append(f.trades[c.PortfolioID], c)
	}
	return created, nil
}

func (f *fakeStore) ListTrades(_ context.Context, portfolioID string) ([]storage.Trade, error) {
	return f.trades[portfolioID], nil
}



func (f *fakeStore) ImportReport(_ context.Context, portfolioID, importID string, trades []storage.Trade, cash []storage.CashOperation) (storage.ImportResult, error) {
	var res storage.ImportResult
	for _, t := range trades {
		if t.SecID == "UNKNOWN" {
			return storage.ImportResult{}, storage.ErrUnknownInstrument
		}
	}
	known := map[string]bool{}
	for _, t := range f.trades[portfolioID] {
		known["t"+t.ExternalID] = t.ExternalID != ""
	}
	for _, c := range f.cash[portfolioID] {
		known["c"+c.ExternalID] = c.ExternalID != ""
	}
	for _, t := range trades {
		if known["t"+t.ExternalID] {
			res.TradesSkipped++
			continue
		}
		t.ID = f.genID("t")
		f.trades[portfolioID] = append(f.trades[portfolioID], t)
		known["t"+t.ExternalID] = t.ExternalID != ""
		res.TradesCreated++
	}
	for _, c := range cash {
		if known["c"+c.ExternalID] {
			res.CashSkipped++
			continue
		}
		c.ID = f.genID("c")
		f.cash[portfolioID] = append(f.cash[portfolioID], c)
		known["c"+c.ExternalID] = c.ExternalID != ""
		res.CashCreated++
	}
	if r, ok := f.imports[importID]; ok && r.PortfolioID == portfolioID &&
		(r.Status == storage.ImportQueued || r.Status == storage.ImportProcessing) {
		r.Status = storage.ImportDone
		r.TradesCreated, r.TradesSkipped = res.TradesCreated, res.TradesSkipped
		r.CashCreated, r.CashSkipped = res.CashCreated, res.CashSkipped
		f.imports[importID] = r
	}
	return res, nil
}

func (f *fakeStore) ListCashOperations(_ context.Context, portfolioID string) ([]storage.CashOperation, error) {
	return f.cash[portfolioID], nil
}

func (f *fakeStore) LatestPrices(_ context.Context, instruments [][2]string) (map[string]float64, error) {
	out := make(map[string]float64, len(instruments))
	for _, inst := range instruments {
		key := inst[0] + "/" + inst[1]
		if p, ok := f.prices[key]; ok {
			out[key] = p
		}
	}
	return out, nil
}

func newTestServer(store Store) *Server {
	return &Server{Store: store, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func withUserID(id string) context.Context {
	md := metadata.Pairs(authmd.UserIDKey, id)
	return metadata.NewIncomingContext(context.Background(), md)
}

func TestCreatePortfolio_DefaultsName(t *testing.T) {
	s := newTestServer(newFakeStore())
	p, err := s.CreatePortfolio(withUserID("u1"), &portfoliopb.CreatePortfolioRequest{})
	if err != nil {
		t.Fatalf("CreatePortfolio: %v", err)
	}
	if p.Name != "Основной" {
		t.Errorf("name = %q, want default", p.Name)
	}
}

func TestCreatePortfolio_RequiresMetadata(t *testing.T) {
	s := newTestServer(newFakeStore())
	_, err := s.CreatePortfolio(context.Background(), &portfoliopb.CreatePortfolioRequest{Name: "x"})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestListPortfolios_OnlyOwn(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	if _, err := s.CreatePortfolio(withUserID("u1"), &portfoliopb.CreatePortfolioRequest{Name: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatePortfolio(withUserID("u2"), &portfoliopb.CreatePortfolioRequest{Name: "b"}); err != nil {
		t.Fatal(err)
	}

	resp, err := s.ListPortfolios(withUserID("u1"), &emptypb.Empty{})
	if err != nil {
		t.Fatalf("ListPortfolios: %v", err)
	}
	if len(resp.Portfolios) != 1 || resp.Portfolios[0].Name != "a" {
		t.Errorf("unexpected list: %+v", resp.Portfolios)
	}
}

func TestGetPortfolio_NotYoursIsNotFound(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	p, err := s.CreatePortfolio(withUserID("owner"), &portfoliopb.CreatePortfolioRequest{Name: "a"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.GetPortfolio(withUserID("someone-else"), &portfoliopb.GetPortfolioRequest{Id: p.Id})
	if status.Code(err) != codes.NotFound {
		t.Errorf("code = %v, want NotFound (ownership must not leak as a different error)", status.Code(err))
	}
}

func TestGetPortfolio_NonexistentIsNotFound(t *testing.T) {
	s := newTestServer(newFakeStore())
	_, err := s.GetPortfolio(withUserID("u1"), &portfoliopb.GetPortfolioRequest{Id: "nope"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("code = %v, want NotFound", status.Code(err))
	}
}

func TestUpdatePortfolio_Renames(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	p, err := s.CreatePortfolio(withUserID("u1"), &portfoliopb.CreatePortfolioRequest{Name: "old"})
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.UpdatePortfolio(withUserID("u1"), &portfoliopb.UpdatePortfolioRequest{Id: p.Id, Name: "  ИИС  "})
	if err != nil {
		t.Fatalf("UpdatePortfolio: %v", err)
	}
	if got.Name != "ИИС" || store.portfolios[p.Id].Name != "ИИС" {
		t.Errorf("name = %q (stored %q), want trimmed \"ИИС\"", got.Name, store.portfolios[p.Id].Name)
	}
}

func TestUpdatePortfolio_Validation(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	p, err := s.CreatePortfolio(withUserID("u1"), &portfoliopb.CreatePortfolioRequest{Name: "a"})
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"", "   ", strings.Repeat("я", maxPortfolioNameLen+1)} {
		_, err := s.UpdatePortfolio(withUserID("u1"), &portfoliopb.UpdatePortfolioRequest{Id: p.Id, Name: name})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("name len %d: code = %v, want InvalidArgument", len(name), status.Code(err))
		}
	}
	if _, err := s.UpdatePortfolio(withUserID("u1"), &portfoliopb.UpdatePortfolioRequest{
		Id: p.Id, Name: strings.Repeat("я", maxPortfolioNameLen),
	}); err != nil {
		t.Errorf("name of exactly %d runes rejected: %v", maxPortfolioNameLen, err)
	}
}

func TestUpdatePortfolio_NotYoursIsNotFound(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	p, err := s.CreatePortfolio(withUserID("owner"), &portfoliopb.CreatePortfolioRequest{Name: "a"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.UpdatePortfolio(withUserID("someone-else"), &portfoliopb.UpdatePortfolioRequest{Id: p.Id, Name: "b"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("code = %v, want NotFound", status.Code(err))
	}
	if store.portfolios[p.Id].Name != "a" {
		t.Errorf("foreign portfolio was renamed to %q", store.portfolios[p.Id].Name)
	}
}

func TestCreateTrade_ValidatesSide(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	p, err := s.CreatePortfolio(withUserID("u1"), &portfoliopb.CreatePortfolioRequest{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.CreateTrade(withUserID("u1"), &portfoliopb.CreateTradeRequest{
		PortfolioId: p.Id, Secid: "SBER", Board: "TQBR", Side: "hold", Quantity: 1, Price: 1,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestCreateTrade_UnknownInstrument(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	p, err := s.CreatePortfolio(withUserID("u1"), &portfoliopb.CreatePortfolioRequest{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.CreateTrade(withUserID("u1"), &portfoliopb.CreateTradeRequest{
		PortfolioId: p.Id, Secid: "UNKNOWN", Board: "TQBR", Side: "buy", Quantity: 1, Price: 1,
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", status.Code(err))
	}
}

func TestCreateTrade_RejectsOthersPortfolio(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	p, err := s.CreatePortfolio(withUserID("owner"), &portfoliopb.CreatePortfolioRequest{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.CreateTrade(withUserID("attacker"), &portfoliopb.CreateTradeRequest{
		PortfolioId: p.Id, Secid: "SBER", Board: "TQBR", Side: "buy", Quantity: 1, Price: 1,
	})
	if status.Code(err) != codes.NotFound {
		t.Errorf("code = %v, want NotFound", status.Code(err))
	}
}

func TestCreateTrades_EmptyBatchRejected(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	p, err := s.CreatePortfolio(withUserID("u1"), &portfoliopb.CreatePortfolioRequest{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.CreateTrades(withUserID("u1"), &portfoliopb.CreateTradesRequest{PortfolioId: p.Id})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestCreateTrades_CreatesAllInOrder(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	p, err := s.CreatePortfolio(withUserID("u1"), &portfoliopb.CreatePortfolioRequest{})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := s.CreateTrades(withUserID("u1"), &portfoliopb.CreateTradesRequest{
		PortfolioId: p.Id,
		Trades: []*portfoliopb.TradeInput{
			{Secid: "SBER", Board: "TQBR", Side: "buy", Quantity: 10, Price: 100},
			{Secid: "LKOH", Board: "TQBR", Side: "buy", Quantity: 2, Price: 5000, Fee: 10, Currency: "RUB"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTrades: %v", err)
	}
	if len(resp.Trades) != 2 {
		t.Fatalf("created %d trades, want 2", len(resp.Trades))
	}
	if resp.Trades[0].Secid != "SBER" || resp.Trades[1].Secid != "LKOH" {
		t.Errorf("order not preserved: %s, %s", resp.Trades[0].Secid, resp.Trades[1].Secid)
	}

	stored, err := s.ListTrades(withUserID("u1"), &portfoliopb.ListTradesRequest{PortfolioId: p.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Trades) != 2 {
		t.Errorf("stored %d trades, want 2", len(stored.Trades))
	}
}

func TestCreateTrades_BadEntryRejectsWholeBatch(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	p, err := s.CreatePortfolio(withUserID("u1"), &portfoliopb.CreatePortfolioRequest{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.CreateTrades(withUserID("u1"), &portfoliopb.CreateTradesRequest{
		PortfolioId: p.Id,
		Trades: []*portfoliopb.TradeInput{
			{Secid: "SBER", Board: "TQBR", Side: "buy", Quantity: 10, Price: 100},
			{Secid: "GAZP", Board: "TQBR", Side: "sell", Quantity: 0, Price: 100},
		},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}

	stored, err := s.ListTrades(withUserID("u1"), &portfoliopb.ListTradesRequest{PortfolioId: p.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Trades) != 0 {
		t.Errorf("valid first trade must not be stored when the batch is rejected, got %d", len(stored.Trades))
	}
}

func TestCreateTrades_UnknownInstrumentRejectsWholeBatch(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	p, err := s.CreatePortfolio(withUserID("u1"), &portfoliopb.CreatePortfolioRequest{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.CreateTrades(withUserID("u1"), &portfoliopb.CreateTradesRequest{
		PortfolioId: p.Id,
		Trades: []*portfoliopb.TradeInput{
			{Secid: "SBER", Board: "TQBR", Side: "buy", Quantity: 10, Price: 100},
			{Secid: "UNKNOWN", Board: "TQBR", Side: "buy", Quantity: 1, Price: 1},
		},
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", status.Code(err))
	}

	stored, err := s.ListTrades(withUserID("u1"), &portfoliopb.ListTradesRequest{PortfolioId: p.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Trades) != 0 {
		t.Errorf("nothing must be stored when the batch is rolled back, got %d", len(stored.Trades))
	}
}

func TestCreateTrades_RejectsOthersPortfolio(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	p, err := s.CreatePortfolio(withUserID("owner"), &portfoliopb.CreatePortfolioRequest{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.CreateTrades(withUserID("attacker"), &portfoliopb.CreateTradesRequest{
		PortfolioId: p.Id,
		Trades: []*portfoliopb.TradeInput{
			{Secid: "SBER", Board: "TQBR", Side: "buy", Quantity: 1, Price: 1},
		},
	})
	if status.Code(err) != codes.NotFound {
		t.Errorf("code = %v, want NotFound", status.Code(err))
	}
}

func TestGetHoldingsAndPnL(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	p, err := s.CreatePortfolio(withUserID("u1"), &portfoliopb.CreatePortfolioRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTrade(withUserID("u1"), &portfoliopb.CreateTradeRequest{
		PortfolioId: p.Id, Secid: "sber", Board: "tqbr", Side: "buy", Quantity: 10, Price: 100,
	}); err != nil {
		t.Fatal(err)
	}
	store.prices["SBER/TQBR"] = 120

	holdings, err := s.GetHoldings(withUserID("u1"), &portfoliopb.GetHoldingsRequest{PortfolioId: p.Id})
	if err != nil {
		t.Fatalf("GetHoldings: %v", err)
	}
	if len(holdings.Holdings) != 1 || holdings.Holdings[0].Quantity != 10 {
		t.Fatalf("unexpected holdings: %+v", holdings.Holdings)
	}
	if holdings.Holdings[0].GetCurrentPrice() != 120 {
		t.Errorf("current_price = %v, want 120", holdings.Holdings[0].GetCurrentPrice())
	}

	summary, err := s.GetPnL(withUserID("u1"), &portfoliopb.GetPnLRequest{PortfolioId: p.Id})
	if err != nil {
		t.Fatalf("GetPnL: %v", err)
	}
	wantUnrealized := 10 * (120 - 100.0)
	if summary.TotalUnrealizedPnl != wantUnrealized {
		t.Errorf("unrealized pnl = %v, want %v", summary.TotalUnrealizedPnl, wantUnrealized)
	}
}

func importReq(portfolioID string) *portfoliopb.ImportReportRequest {
	at := timestamppb.New(time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC))
	return &portfoliopb.ImportReportRequest{
		PortfolioId: portfolioID,
		Trades: []*portfoliopb.TradeInput{
			{Secid: "sber", Board: "tqbr", Side: "buy", Quantity: 10, Price: 300, Fee: 1, ExecutedAt: at, ExternalId: "sber:A:trade:1"},
			{Secid: "SU26254RMFS1", Board: "TQOB", Side: "buy", Quantity: 3, Price: 841, Fee: 7.83, AccruedInterest: 82.26, ExecutedAt: at, ExternalId: "sber:A:trade:2"},
		},
		CashOperations: []*portfoliopb.CashOperationInput{
			{Type: "deposit", Amount: 6000, OccurredAt: at, ExternalId: "sber:A:cash:1"},
			{Type: "dividend", Amount: 300, Secid: "SBER", Board: "TQBR", OccurredAt: at, ExternalId: "sber:A:cash:2"},
		},
	}
}

func TestImportReport_IdempotentAndFeedsPnL(t *testing.T) {
	store := newFakeStore()
	srv := newTestServer(store)
	ctx := withUserID("u1")
	p, _ := srv.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{})

	res, err := srv.ImportReport(ctx, importReq(p.GetId()))
	if err != nil {
		t.Fatalf("ImportReport: %v", err)
	}
	if res.GetTradesCreated() != 2 || res.GetCashOperationsCreated() != 2 {
		t.Errorf("first import = %+v", res)
	}

	res, err = srv.ImportReport(ctx, importReq(p.GetId()))
	if err != nil {
		t.Fatalf("second ImportReport: %v", err)
	}
	if res.GetTradesCreated() != 0 || res.GetTradesSkipped() != 2 || res.GetCashOperationsSkipped() != 2 {
		t.Errorf("second import must skip everything, got %+v", res)
	}

	trades, _ := srv.ListTrades(ctx, &portfoliopb.ListTradesRequest{PortfolioId: p.GetId()})
	if len(trades.GetTrades()) != 2 || trades.GetTrades()[0].GetSecid() != "SBER" || trades.GetTrades()[1].GetAccruedInterest() != 82.26 {
		t.Errorf("trades = %+v", trades.GetTrades())
	}

	store.prices["SBER/TQBR"] = 320
	sum, err := srv.GetPnL(ctx, &portfoliopb.GetPnLRequest{PortfolioId: p.GetId()})
	if err != nil {
		t.Fatalf("GetPnL: %v", err)
	}
	if sum.GetTotalDividends() != 300 || sum.GetNetDeposits() != 6000 || sum.GetTotalAccruedInterest() != -82.26 {
		t.Errorf("summary = %+v", sum)
	}
	
	if d := sum.GetTotalPnl() - 416.74; d > 1e-6 || d < -1e-6 {
		t.Errorf("total pnl = %v, want 416.74", sum.GetTotalPnl())
	}
}

func TestImportReport_Validation(t *testing.T) {
	srv := newTestServer(newFakeStore())
	ctx := withUserID("u1")
	p, _ := srv.CreatePortfolio(ctx, &portfoliopb.CreatePortfolioRequest{})

	_, err := srv.ImportReport(ctx, &portfoliopb.ImportReportRequest{PortfolioId: p.GetId()})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("empty report: code = %v, want InvalidArgument", status.Code(err))
	}

	req := importReq(p.GetId())
	req.CashOperations[0].Type = "gift"
	if _, err := srv.ImportReport(ctx, req); status.Code(err) != codes.InvalidArgument {
		t.Errorf("bad cash type: code = %v, want InvalidArgument", status.Code(err))
	}

	req = importReq(p.GetId())
	req.Trades[0].Secid = "UNKNOWN"
	if _, err := srv.ImportReport(ctx, req); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("unknown instrument: code = %v, want FailedPrecondition", status.Code(err))
	}

	if _, err := srv.ImportReport(withUserID("u2"), importReq(p.GetId())); status.Code(err) != codes.NotFound {
		t.Errorf("other user's portfolio: code = %v, want NotFound", status.Code(err))
	}
}
