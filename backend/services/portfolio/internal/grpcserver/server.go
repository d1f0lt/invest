package grpcserver

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"invest/backend/services/portfolio/internal/authmd"
	"invest/backend/services/portfolio/internal/pnl"
	"invest/backend/services/portfolio/internal/storage"
	portfoliopb "invest/backend/services/portfolio/proto"
)

type Store interface {
	CreatePortfolio(ctx context.Context, userID, name string, memberIDs []string) (storage.Portfolio, error)
	ListPortfoliosByUser(ctx context.Context, userID string) ([]storage.Portfolio, error)
	GetPortfolio(ctx context.Context, id string) (storage.Portfolio, error)
	RenamePortfolio(ctx context.Context, id, name string) (storage.Portfolio, error)
	CreateTrade(ctx context.Context, t storage.Trade) (storage.Trade, error)
	CreateTradesBatch(ctx context.Context, trades []storage.Trade) ([]storage.Trade, error)
	ListTrades(ctx context.Context, portfolioIDs []string) ([]storage.Trade, error)
	ImportReport(ctx context.Context, portfolioID, importID string, trades []storage.Trade, cash []storage.CashOperation, opening *storage.OpeningScope) (storage.ImportResult, error)
	ListCashOperations(ctx context.Context, portfolioIDs []string) ([]storage.CashOperation, error)
	LatestPrices(ctx context.Context, instruments [][2]string) (map[string]float64, error)
	PrevCloses(ctx context.Context, instruments [][2]string) (map[string]float64, error)
	DailyCloses(ctx context.Context, instruments [][2]string, from time.Time) (map[string][]storage.DailyClose, error)

	ListBrokers(ctx context.Context) ([]storage.Broker, error)
	GetBroker(ctx context.Context, id string) (storage.Broker, error)
	CreateReportImport(ctx context.Context, r storage.ReportImport) (storage.ReportImport, error)
	GetReportImport(ctx context.Context, id string) (storage.ReportImport, error)
	ListReportImports(ctx context.Context, portfolioIDs []string) ([]storage.ReportImport, error)
	UpdateReportImportStatus(ctx context.Context, id, status, errMsg string) (storage.ReportImport, error)
}

type Server struct {
	portfoliopb.UnimplementedPortfolioServiceServer

	Store Store
	Log   *slog.Logger

	Loc *time.Location
	Now func() time.Time
}

func (s *Server) CreatePortfolio(ctx context.Context, req *portfoliopb.CreatePortfolioRequest) (*portfoliopb.Portfolio, error) {
	userID, err := authmd.UserID(ctx)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(req.GetName())
	if utf8.RuneCountInString(name) > maxPortfolioNameLen {
		return nil, status.Errorf(codes.InvalidArgument, "name must be at most %d characters", maxPortfolioNameLen)
	}

	var members []string
	if len(req.GetMemberIds()) > 0 {
		members, err = s.validateMembers(ctx, userID, req.GetMemberIds())
		if err != nil {
			return nil, err
		}
	}

	if name == "" {
		name = "Основной"
		if members != nil {
			name = "Составной"
		}
	}

	p, err := s.Store.CreatePortfolio(ctx, userID, name, members)
	if err != nil {
		s.Log.Error("create portfolio", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	return toPortfolioPB(p), nil
}

const minCompositeMembers = 2

func (s *Server) validateMembers(ctx context.Context, userID string, ids []string) ([]string, error) {
	seen := map[string]bool{}
	var members []string
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true

		p, err := s.Store.GetPortfolio(ctx, id)
		if errors.Is(err, storage.ErrNotFound) || (err == nil && p.UserID != userID) {
			return nil, status.Error(codes.NotFound, "Портфель для составного не найден")
		}
		if err != nil {
			s.Log.Error("get member portfolio", "error", err)
			return nil, status.Error(codes.Internal, "internal error")
		}
		if p.Composite() {
			return nil, status.Error(codes.InvalidArgument, "Составной портфель нельзя включить в другой составной")
		}
		members = append(members, p.ID)
	}
	if len(members) < minCompositeMembers {
		return nil, status.Errorf(codes.InvalidArgument, "Выберите хотя бы %d портфеля", minCompositeMembers)
	}
	return members, nil
}

func (s *Server) ListPortfolios(ctx context.Context, _ *emptypb.Empty) (*portfoliopb.ListPortfoliosResponse, error) {
	userID, err := authmd.UserID(ctx)
	if err != nil {
		return nil, err
	}

	list, err := s.Store.ListPortfoliosByUser(ctx, userID)
	if err != nil {
		s.Log.Error("list portfolios", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	out := make([]*portfoliopb.Portfolio, 0, len(list))
	for _, p := range list {
		out = append(out, toPortfolioPB(p))
	}
	return &portfoliopb.ListPortfoliosResponse{Portfolios: out}, nil
}

func (s *Server) GetPortfolio(ctx context.Context, req *portfoliopb.GetPortfolioRequest) (*portfoliopb.Portfolio, error) {
	p, err := s.loadOwnedPortfolio(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return toPortfolioPB(p), nil
}

const maxPortfolioNameLen = 100

func (s *Server) UpdatePortfolio(ctx context.Context, req *portfoliopb.UpdatePortfolioRequest) (*portfoliopb.Portfolio, error) {
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if utf8.RuneCountInString(name) > maxPortfolioNameLen {
		return nil, status.Errorf(codes.InvalidArgument, "name must be at most %d characters", maxPortfolioNameLen)
	}

	if _, err := s.loadOwnedPortfolio(ctx, req.GetId()); err != nil {
		return nil, err
	}

	p, err := s.Store.RenamePortfolio(ctx, req.GetId(), name)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "portfolio not found")
	}
	if err != nil {
		s.Log.Error("rename portfolio", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	return toPortfolioPB(p), nil
}

func (s *Server) CreateTrade(ctx context.Context, req *portfoliopb.CreateTradeRequest) (*portfoliopb.Trade, error) {
	p, err := s.loadWritablePortfolio(ctx, req.GetPortfolioId())
	if err != nil {
		return nil, err
	}

	t, err := s.validateTrade(&portfoliopb.TradeInput{
		Secid:      req.GetSecid(),
		Board:      req.GetBoard(),
		Side:       req.GetSide(),
		Quantity:   req.GetQuantity(),
		Price:      req.GetPrice(),
		Fee:        req.GetFee(),
		Currency:   req.GetCurrency(),
		ExecutedAt: req.ExecutedAt,
	})
	if err != nil {
		return nil, err
	}
	t.PortfolioID = p.ID

	trade, err := s.Store.CreateTrade(ctx, t)
	if err != nil {

		return nil, s.insertError("create trade", err)
	}
	return toTradePB(trade), nil
}

func (s *Server) validateTrade(t *portfoliopb.TradeInput) (storage.Trade, error) {
	side := strings.ToLower(strings.TrimSpace(t.GetSide()))
	if side != "buy" && side != "sell" {
		return storage.Trade{}, status.Error(codes.InvalidArgument, `side must be "buy" or "sell"`)
	}
	if t.GetSecid() == "" || t.GetBoard() == "" {
		return storage.Trade{}, status.Error(codes.InvalidArgument, "secid and board are required")
	}
	if t.GetQuantity() <= 0 {
		return storage.Trade{}, status.Error(codes.InvalidArgument, "quantity must be > 0")
	}
	if t.GetPrice() < 0 || t.GetFee() < 0 || t.GetAccruedInterest() < 0 {
		return storage.Trade{}, status.Error(codes.InvalidArgument, "price, fee and accrued_interest must be >= 0")
	}
	currency := strings.ToUpper(strings.TrimSpace(t.GetCurrency()))
	if currency == "" {
		currency = "RUB"
	}
	executedAt := time.Now()
	if t.ExecutedAt != nil {
		executedAt = t.GetExecutedAt().AsTime()
	}
	return storage.Trade{
		SecID:      strings.ToUpper(strings.TrimSpace(t.GetSecid())),
		Board:      strings.ToUpper(strings.TrimSpace(t.GetBoard())),
		Side:       side,
		Quantity:   t.GetQuantity(),
		Price:      t.GetPrice(),
		Fee:        t.GetFee(),
		Currency:   currency,
		ExecutedAt: executedAt,

		AccruedInterest: t.GetAccruedInterest(),
		ExternalID:      strings.TrimSpace(t.GetExternalId()),
		SecurityName:    strings.TrimSpace(t.GetSecurityName()),
		ISIN:            strings.ToUpper(strings.TrimSpace(t.GetIsin())),
	}, nil
}

func (s *Server) CreateTrades(ctx context.Context, req *portfoliopb.CreateTradesRequest) (*portfoliopb.CreateTradesResponse, error) {
	p, err := s.loadWritablePortfolio(ctx, req.GetPortfolioId())
	if err != nil {
		return nil, err
	}

	inputs := req.GetTrades()
	if len(inputs) == 0 {
		return nil, status.Error(codes.InvalidArgument, "trades must not be empty")
	}

	stored := make([]storage.Trade, 0, len(inputs))
	for _, in := range inputs {
		t, err := s.validateTrade(in)
		if err != nil {
			return nil, err
		}
		t.PortfolioID = p.ID
		stored = append(stored, t)
	}

	created, err := s.Store.CreateTradesBatch(ctx, stored)
	if err != nil {

		return nil, s.insertError("create trades batch", err)
	}

	out := make([]*portfoliopb.Trade, 0, len(created))
	for _, t := range created {
		out = append(out, toTradePB(t))
	}
	return &portfoliopb.CreateTradesResponse{Trades: out}, nil
}

func (s *Server) ListTrades(ctx context.Context, req *portfoliopb.ListTradesRequest) (*portfoliopb.ListTradesResponse, error) {
	p, err := s.loadOwnedPortfolio(ctx, req.GetPortfolioId())
	if err != nil {
		return nil, err
	}
	trades, err := s.Store.ListTrades(ctx, p.SourceIDs())
	if err != nil {
		s.Log.Error("list trades", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	out := make([]*portfoliopb.Trade, 0, len(trades))
	for _, t := range trades {
		out = append(out, toTradePB(t))
	}
	return &portfoliopb.ListTradesResponse{Trades: out}, nil
}

func (s *Server) GetHoldings(ctx context.Context, req *portfoliopb.GetHoldingsRequest) (*portfoliopb.GetHoldingsResponse, error) {
	summary, err := s.computePnL(ctx, req.GetPortfolioId())
	if err != nil {
		return nil, err
	}
	open := pnl.Holdings(summary)
	out := make([]*portfoliopb.Holding, 0, len(open))
	for _, i := range open {
		out = append(out, toHoldingPB(i))
	}
	return &portfoliopb.GetHoldingsResponse{Holdings: out}, nil
}

func (s *Server) GetPnL(ctx context.Context, req *portfoliopb.GetPnLRequest) (*portfoliopb.PnLSummary, error) {
	summary, err := s.computePnL(ctx, req.GetPortfolioId())
	if err != nil {
		return nil, err
	}
	instruments := make([]*portfoliopb.InstrumentPnL, 0, len(summary.Instruments))
	for _, i := range summary.Instruments {
		instruments = append(instruments, toInstrumentPB(i))
	}
	return &portfoliopb.PnLSummary{
		Instruments:          instruments,
		TotalRealizedPnl:     summary.TotalRealizedPnL,
		TotalUnrealizedPnl:   summary.TotalUnrealizedPnL,
		TotalPnl:             summary.TotalPnL(),
		TotalDividends:       summary.TotalDividends,
		TotalCoupons:         summary.TotalCoupons,
		TotalAccruedInterest: summary.TotalAccruedInterest,
		TotalTaxes:           summary.TotalTaxes,
		TotalFees:            summary.TotalFees,
		TotalOther:           summary.TotalOther,
		NetDeposits:          summary.NetDeposits,
		CashBalance:          summary.CashBalance,
		TotalDayChange:       summary.TotalDayChange,
	}, nil
}

func (s *Server) loadOwnedPortfolio(ctx context.Context, id string) (storage.Portfolio, error) {
	userID, err := authmd.UserID(ctx)
	if err != nil {
		return storage.Portfolio{}, err
	}

	p, err := s.Store.GetPortfolio(ctx, id)
	if errors.Is(err, storage.ErrNotFound) || (err == nil && p.UserID != userID) {
		return storage.Portfolio{}, status.Error(codes.NotFound, "portfolio not found")
	}
	if err != nil {
		s.Log.Error("get portfolio", "error", err)
		return storage.Portfolio{}, status.Error(codes.Internal, "internal error")
	}
	return p, nil
}

func (s *Server) loadWritablePortfolio(ctx context.Context, id string) (storage.Portfolio, error) {
	p, err := s.loadOwnedPortfolio(ctx, id)
	if err != nil {
		return storage.Portfolio{}, err
	}
	if p.Composite() {
		return storage.Portfolio{}, status.Error(codes.FailedPrecondition,
			"Составной портфель собирается из других портфелей — загрузите отчёт в один из них")
	}
	return p, nil
}

type ledger struct {
	trades []pnl.Trade
	cash   []pnl.CashFlow
}

type book struct {
	ledgers     []ledger
	instruments [][2]string
	prices      map[string]float64
}

func (s *Server) loadBook(ctx context.Context, portfolioID string) (book, error) {
	p, err := s.loadOwnedPortfolio(ctx, portfolioID)
	if err != nil {
		return book{}, err
	}

	var b book
	seen := map[[2]string]bool{}
	for _, id := range p.SourceIDs() {
		storedTrades, err := s.Store.ListTrades(ctx, []string{id})
		if err != nil {
			s.Log.Error("list trades", "error", err)
			return book{}, status.Error(codes.Internal, "internal error")
		}
		storedCash, err := s.Store.ListCashOperations(ctx, []string{id})
		if err != nil {
			s.Log.Error("list cash operations", "error", err)
			return book{}, status.Error(codes.Internal, "internal error")
		}

		var l ledger
		l.cash = make([]pnl.CashFlow, 0, len(storedCash))
		for _, c := range storedCash {
			l.cash = append(l.cash, pnl.CashFlow{
				Type: c.Type, Amount: c.Amount, SecID: c.SecID, Board: c.Board, OccurredAt: c.OccurredAt,
			})
		}
		l.trades = make([]pnl.Trade, 0, len(storedTrades))
		for _, t := range storedTrades {
			l.trades = append(l.trades, pnl.Trade{
				SecID: t.SecID, Board: t.Board, Side: pnl.TradeSide(t.Side),
				Quantity: t.Quantity, Price: t.Price, Fee: t.Fee, ExecutedAt: t.ExecutedAt,
				AccruedInterest: t.AccruedInterest,
			})
			key := [2]string{t.SecID, t.Board}
			if !seen[key] {
				seen[key] = true
				b.instruments = append(b.instruments, key)
			}
		}
		b.ledgers = append(b.ledgers, l)
	}

	b.prices, err = s.Store.LatestPrices(ctx, b.instruments)
	if err != nil {
		s.Log.Error("latest prices", "error", err)
		return book{}, status.Error(codes.Internal, "internal error")
	}
	return b, nil
}

func (s *Server) computePnL(ctx context.Context, portfolioID string) (pnl.Summary, error) {
	b, err := s.loadBook(ctx, portfolioID)
	if err != nil {
		return pnl.Summary{}, err
	}

	prevCloses, err := s.Store.PrevCloses(ctx, b.instruments)
	if err != nil {
		s.Log.Error("prev closes", "error", err)
		return pnl.Summary{}, status.Error(codes.Internal, "internal error")
	}

	summaries := make([]pnl.Summary, 0, len(b.ledgers))
	for _, l := range b.ledgers {
		summary := pnl.Compute(l.trades, l.cash, b.prices)
		pnl.ApplyDayChange(&summary, prevCloses)
		summaries = append(summaries, summary)
	}
	return pnl.Merge(summaries), nil
}

func toPortfolioPB(p storage.Portfolio) *portfoliopb.Portfolio {
	return &portfoliopb.Portfolio{
		Id:        p.ID,
		Name:      p.Name,
		CreatedAt: timestamppb.New(p.CreatedAt),
		UpdatedAt: timestamppb.New(p.UpdatedAt),
		MemberIds: p.MemberIDs,
	}
}

func toTradePB(t storage.Trade) *portfoliopb.Trade {
	return &portfoliopb.Trade{
		Id:         t.ID,
		Secid:      t.SecID,
		Board:      t.Board,
		Side:       t.Side,
		Quantity:   t.Quantity,
		Price:      t.Price,
		Fee:        t.Fee,
		Currency:   t.Currency,
		ExecutedAt: timestamppb.New(t.ExecutedAt),

		AccruedInterest: t.AccruedInterest,
		ExternalId:      t.ExternalID,
	}
}

func toCashOperationPB(c storage.CashOperation) *portfoliopb.CashOperation {
	return &portfoliopb.CashOperation{
		Id:          c.ID,
		Type:        c.Type,
		Amount:      c.Amount,
		Currency:    c.Currency,
		OccurredAt:  timestamppb.New(c.OccurredAt),
		Secid:       c.SecID,
		Board:       c.Board,
		Description: c.Description,
		ExternalId:  c.ExternalID,
	}
}

func toHoldingPB(i pnl.InstrumentPnL) *portfoliopb.Holding {
	return &portfoliopb.Holding{
		Secid:         i.SecID,
		Board:         i.Board,
		Quantity:      i.Quantity,
		AvgCost:       i.AvgCost,
		CurrentPrice:  i.CurrentPrice,
		MarketValue:   i.MarketValue,
		UnrealizedPnl: i.UnrealizedPnL,
		DayChange:     i.DayChange,
	}
}

func toInstrumentPB(i pnl.InstrumentPnL) *portfoliopb.InstrumentPnL {
	return &portfoliopb.InstrumentPnL{
		Holding:         toHoldingPB(i),
		RealizedPnl:     i.RealizedPnL,
		TotalPnl:        i.TotalPnL(),
		Dividends:       i.Dividends,
		Coupons:         i.Coupons,
		AccruedInterest: i.AccruedInterest,
	}
}

func (s *Server) insertError(op string, err error) error {
	switch {
	case errors.Is(err, storage.ErrUnknownInstrument):
		return status.Errorf(codes.FailedPrecondition, "unknown secid/board (price_updater hasn't seen this instrument): %v", err)
	case errors.Is(err, storage.ErrDuplicate):
		return status.Errorf(codes.AlreadyExists, "%v", err)
	}
	s.Log.Error(op, "error", err)
	return status.Error(codes.Internal, "internal error")
}

var cashTypes = map[string]bool{
	pnl.CashDeposit: true, pnl.CashWithdrawal: true, pnl.CashDividend: true, pnl.CashCoupon: true,
	pnl.CashRedemption: true, pnl.CashTax: true, pnl.CashFee: true, pnl.CashOther: true,
}

func validateCashOperation(c *portfoliopb.CashOperationInput) (storage.CashOperation, error) {
	typ := strings.ToLower(strings.TrimSpace(c.GetType()))
	if !cashTypes[typ] {
		return storage.CashOperation{}, status.Errorf(codes.InvalidArgument, "unknown cash operation type %q", c.GetType())
	}
	if c.GetAmount() == 0 {
		return storage.CashOperation{}, status.Error(codes.InvalidArgument, "cash operation amount must not be 0")
	}
	if c.GetOccurredAt() == nil {
		return storage.CashOperation{}, status.Error(codes.InvalidArgument, "cash operation occurred_at is required")
	}
	currency := strings.ToUpper(strings.TrimSpace(c.GetCurrency()))
	if currency == "" {
		currency = "RUB"
	}
	return storage.CashOperation{
		Type:        typ,
		Amount:      c.GetAmount(),
		Currency:    currency,
		OccurredAt:  c.GetOccurredAt().AsTime(),
		SecID:       strings.ToUpper(strings.TrimSpace(c.GetSecid())),
		Board:       strings.ToUpper(strings.TrimSpace(c.GetBoard())),
		Description: strings.TrimSpace(c.GetDescription()),
		ExternalID:  strings.TrimSpace(c.GetExternalId()),
	}, nil
}

func (s *Server) ImportReport(ctx context.Context, req *portfoliopb.ImportReportRequest) (*portfoliopb.ImportReportResponse, error) {
	p, err := s.loadWritablePortfolio(ctx, req.GetPortfolioId())
	if err != nil {
		return nil, err
	}
	if len(req.GetTrades()) == 0 && len(req.GetCashOperations()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "report is empty: no trades and no cash operations")
	}

	trades := make([]storage.Trade, 0, len(req.GetTrades()))
	for _, in := range req.GetTrades() {
		t, err := s.validateTrade(in)
		if err != nil {
			return nil, err
		}
		t.PortfolioID = p.ID
		trades = append(trades, t)
	}
	cash := make([]storage.CashOperation, 0, len(req.GetCashOperations()))
	for _, in := range req.GetCashOperations() {
		c, err := validateCashOperation(in)
		if err != nil {
			return nil, err
		}
		c.PortfolioID = p.ID
		cash = append(cash, c)
	}

	importID := strings.TrimSpace(req.GetReportImportId())
	if importID != "" {

		imp, err := s.Store.GetReportImport(ctx, importID)
		if errors.Is(err, storage.ErrNotFound) || (err == nil && imp.PortfolioID != p.ID) {
			return nil, status.Error(codes.NotFound, "report import not found")
		}
		if err != nil {
			s.Log.Error("get report import", "error", err)
			return nil, status.Error(codes.Internal, "internal error")
		}
	}

	var opening *storage.OpeningScope
	if key := strings.TrimSpace(req.GetAccountKey()); key != "" && req.GetPeriodStart() != nil {
		opening = &storage.OpeningScope{AccountKey: key, PeriodStart: req.GetPeriodStart().AsTime()}
	}

	res, err := s.Store.ImportReport(ctx, p.ID, importID, trades, cash, opening)
	if err != nil {
		return nil, s.insertError("import report", err)
	}
	return &portfoliopb.ImportReportResponse{
		TradesCreated:         int32(res.TradesCreated),
		TradesSkipped:         int32(res.TradesSkipped),
		CashOperationsCreated: int32(res.CashCreated),
		CashOperationsSkipped: int32(res.CashSkipped),
	}, nil
}

func (s *Server) ListCashOperations(ctx context.Context, req *portfoliopb.ListCashOperationsRequest) (*portfoliopb.ListCashOperationsResponse, error) {
	p, err := s.loadOwnedPortfolio(ctx, req.GetPortfolioId())
	if err != nil {
		return nil, err
	}
	ops, err := s.Store.ListCashOperations(ctx, p.SourceIDs())
	if err != nil {
		s.Log.Error("list cash operations", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	out := make([]*portfoliopb.CashOperation, 0, len(ops))
	for _, c := range ops {
		out = append(out, toCashOperationPB(c))
	}
	return &portfoliopb.ListCashOperationsResponse{CashOperations: out}, nil
}
