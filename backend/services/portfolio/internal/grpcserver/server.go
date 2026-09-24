// Package grpcserver implements the portfolio service's gRPC API
// (invest.portfolio.v1.PortfolioService, see
// proto/portfolio.proto). This is the direct
// replacement for the old internal/httpapi package - same business
// rules, same error semantics (including the "not found" response for a
// portfolio that exists but belongs to someone else - see
// loadOwnedPortfolio), different transport. See architecture-decisions.md,
// "перевод внутреннего взаимодействия сервисов на gRPC".
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

// Store is the subset of storage.Store the gRPC layer needs. Declared as
// an interface here (rather than depending on *storage.Store directly) so
// the server can be tested with a fake.
type Store interface {
	CreatePortfolio(ctx context.Context, userID, name string) (storage.Portfolio, error)
	ListPortfoliosByUser(ctx context.Context, userID string) ([]storage.Portfolio, error)
	GetPortfolio(ctx context.Context, id string) (storage.Portfolio, error)
	RenamePortfolio(ctx context.Context, id, name string) (storage.Portfolio, error)
	CreateTrade(ctx context.Context, t storage.Trade) (storage.Trade, error)
	CreateTradesBatch(ctx context.Context, trades []storage.Trade) ([]storage.Trade, error)
	ListTrades(ctx context.Context, portfolioID string) ([]storage.Trade, error)
	ImportReport(ctx context.Context, portfolioID string, trades []storage.Trade, cash []storage.CashOperation) (storage.ImportResult, error)
	ListCashOperations(ctx context.Context, portfolioID string) ([]storage.CashOperation, error)
	LatestPrices(ctx context.Context, instruments [][2]string) (map[string]float64, error)
}

// Server implements portfoliopb.PortfolioServiceServer.
type Server struct {
	portfoliopb.UnimplementedPortfolioServiceServer

	Store Store
	Log   *slog.Logger
}

func (s *Server) CreatePortfolio(ctx context.Context, req *portfoliopb.CreatePortfolioRequest) (*portfoliopb.Portfolio, error) {
	userID, err := authmd.UserID(ctx)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(req.GetName())
	if name == "" {
		name = "Основной"
	}

	p, err := s.Store.CreatePortfolio(ctx, userID, name)
	if err != nil {
		s.Log.Error("create portfolio", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	return toPortfolioPB(p), nil
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

// maxPortfolioNameLen limits a portfolio name in characters (runes).
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
	p, err := s.loadOwnedPortfolio(ctx, req.GetPortfolioId())
	if err != nil {
		return nil, err
	}

	// Same rules as the batch RPCs (validateTrade) - one place to change.
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
		// The old HTTP API used 422 Unprocessable Entity for an unknown
		// instrument, which has no exact gRPC equivalent; FailedPrecondition
		// is the closest standard code (see insertError).
		return nil, s.insertError("create trade", err)
	}
	return toTradePB(trade), nil
}

// validateTrade applies the same rules CreateTrade applies to a single
// trade, returning a ready gRPC error on failure. Shared by CreateTrade
// and CreateTrades so the batch endpoint can't be used to bypass them.
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
	}, nil
}

// CreateTrades is the batch counterpart of CreateTrade: all trades are
// validated up front, then inserted in one DB transaction - all or
// nothing. This is what the parser service calls after parsing a whole
// broker report, so a retry of the task can't leave half the batch
// applied (the old per-trade loop could, which made requeueing a task
// duplicate already-created trades). Response order matches request
// order.
func (s *Server) CreateTrades(ctx context.Context, req *portfoliopb.CreateTradesRequest) (*portfoliopb.CreateTradesResponse, error) {
	p, err := s.loadOwnedPortfolio(ctx, req.GetPortfolioId())
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
		// Same semantics as CreateTrade; nothing was inserted (single
		// transaction).
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
	trades, err := s.Store.ListTrades(ctx, p.ID)
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
	}, nil
}

// loadOwnedPortfolio loads the portfolio with the given id and checks it
// belongs to the caller (from ctx's x-user-id metadata). A portfolio that
// doesn't exist and a portfolio that exists but belongs to someone else
// are deliberately indistinguishable to the caller (both NotFound) - see
// the old httpapi.loadOwnedPortfolio this replaces.
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

func (s *Server) computePnL(ctx context.Context, portfolioID string) (pnl.Summary, error) {
	p, err := s.loadOwnedPortfolio(ctx, portfolioID)
	if err != nil {
		return pnl.Summary{}, err
	}

	storedTrades, err := s.Store.ListTrades(ctx, p.ID)
	if err != nil {
		s.Log.Error("list trades", "error", err)
		return pnl.Summary{}, status.Error(codes.Internal, "internal error")
	}

	storedCash, err := s.Store.ListCashOperations(ctx, p.ID)
	if err != nil {
		s.Log.Error("list cash operations", "error", err)
		return pnl.Summary{}, status.Error(codes.Internal, "internal error")
	}
	cash := make([]pnl.CashFlow, 0, len(storedCash))
	for _, c := range storedCash {
		cash = append(cash, pnl.CashFlow{
			Type: c.Type, Amount: c.Amount, SecID: c.SecID, Board: c.Board, OccurredAt: c.OccurredAt,
		})
	}

	trades := make([]pnl.Trade, 0, len(storedTrades))
	seen := map[[2]string]bool{}
	var instruments [][2]string
	for _, t := range storedTrades {
		trades = append(trades, pnl.Trade{
			SecID: t.SecID, Board: t.Board, Side: pnl.TradeSide(t.Side),
			Quantity: t.Quantity, Price: t.Price, Fee: t.Fee, ExecutedAt: t.ExecutedAt,
			AccruedInterest: t.AccruedInterest,
		})
		key := [2]string{t.SecID, t.Board}
		if !seen[key] {
			seen[key] = true
			instruments = append(instruments, key)
		}
	}

	prices, err := s.Store.LatestPrices(ctx, instruments)
	if err != nil {
		s.Log.Error("latest prices", "error", err)
		return pnl.Summary{}, status.Error(codes.Internal, "internal error")
	}

	return pnl.Compute(trades, cash, prices), nil
}

func toPortfolioPB(p storage.Portfolio) *portfoliopb.Portfolio {
	return &portfoliopb.Portfolio{
		Id:        p.ID,
		Name:      p.Name,
		CreatedAt: timestamppb.New(p.CreatedAt),
		UpdatedAt: timestamppb.New(p.UpdatedAt),
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

// insertError maps storage insert errors to gRPC codes.
// ErrUnknownInstrument -> FailedPrecondition: the request is well-formed,
// but price_updater hasn't seen this secid/board yet, so the trade can't
// be linked to it. ErrDuplicate -> AlreadyExists.
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

// ImportReport stores a parsed broker report: trades + cash operations,
// one transaction, rows with an already-stored external_id skipped.
func (s *Server) ImportReport(ctx context.Context, req *portfoliopb.ImportReportRequest) (*portfoliopb.ImportReportResponse, error) {
	p, err := s.loadOwnedPortfolio(ctx, req.GetPortfolioId())
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

	res, err := s.Store.ImportReport(ctx, p.ID, trades, cash)
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
	ops, err := s.Store.ListCashOperations(ctx, p.ID)
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
