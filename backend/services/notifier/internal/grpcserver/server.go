package grpcserver

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/url"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"invest/backend/services/notifier/internal/authmd"
	"invest/backend/services/notifier/internal/linktoken"
	"invest/backend/services/notifier/internal/storage"
	notifierpb "invest/backend/services/notifier/proto"
)

type Store interface {
	Instrument(ctx context.Context, secid, board string) (storage.Instrument, error)
	CountAlerts(ctx context.Context, userID string) (int, error)
	CreateAlert(ctx context.Context, a storage.Alert) (storage.Alert, error)
	GetAlert(ctx context.Context, userID, id string) (storage.Alert, error)
	UpdateAlert(ctx context.Context, a storage.Alert) (storage.Alert, error)
	ListAlerts(ctx context.Context, userID string) ([]storage.Alert, error)
	DeleteAlert(ctx context.Context, userID, id string) error
	CreateLinkToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	GetTelegramLink(ctx context.Context, userID string) (storage.TelegramLink, error)
	DeleteTelegramLink(ctx context.Context, userID string) error
}

type BotInfo interface {
	Username() (string, bool)
}

type Server struct {
	notifierpb.UnimplementedNotifierServiceServer

	Store            Store
	Bot              BotInfo
	LinkTokenTTL     time.Duration
	MaxAlertsPerUser int
	Log              *slog.Logger
	Now              func() time.Time
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

const maxPercent = 1000

type target struct {
	price   *float64
	percent *float64
}

func (t target) validate() error {
	switch {
	case t.price != nil:
		if !(*t.price > 0) || math.IsInf(*t.price, 0) {
			return status.Error(codes.InvalidArgument, "target_price must be a positive number")
		}
	case t.percent != nil:
		p := *t.percent
		if math.IsNaN(p) || p == 0 || p <= -100 || p > maxPercent {
			return status.Error(codes.InvalidArgument, "change_percent must be non-zero, greater than -100 and at most 1000")
		}
	default:
		return status.Error(codes.InvalidArgument, "either target_price or change_percent is required")
	}
	return nil
}

func (t target) resolve(in storage.Instrument) (base, targetPrice float64, direction string, err error) {
	if in.LastPrice == nil || !(*in.LastPrice > 0) {
		return 0, 0, "", status.Error(codes.FailedPrecondition, "no current price for this security yet")
	}
	base = *in.LastPrice
	if t.price != nil {
		targetPrice = *t.price
	} else {
		targetPrice = roundTo(base*(1+*t.percent/100), in.Decimals)
	}
	switch {
	case !(targetPrice > 0):
		return 0, 0, "", status.Error(codes.InvalidArgument, "target price must be positive")
	case targetPrice > base:
		direction = storage.DirectionAbove
	case targetPrice < base:
		direction = storage.DirectionBelow
	default:
		return 0, 0, "", status.Error(codes.InvalidArgument, "target price equals the current price")
	}
	return base, targetPrice, direction, nil
}

func createTarget(req *notifierpb.CreateAlertRequest) target {
	switch t := req.GetTarget().(type) {
	case *notifierpb.CreateAlertRequest_TargetPrice:
		return target{price: &t.TargetPrice}
	case *notifierpb.CreateAlertRequest_ChangePercent:
		return target{percent: &t.ChangePercent}
	}
	return target{}
}

func updateTarget(req *notifierpb.UpdateAlertRequest) target {
	switch t := req.GetTarget().(type) {
	case *notifierpb.UpdateAlertRequest_TargetPrice:
		return target{price: &t.TargetPrice}
	case *notifierpb.UpdateAlertRequest_ChangePercent:
		return target{percent: &t.ChangePercent}
	}
	return target{}
}

func (s *Server) CreateAlert(ctx context.Context, req *notifierpb.CreateAlertRequest) (*notifierpb.Alert, error) {
	userID, err := authmd.UserID(ctx)
	if err != nil {
		return nil, err
	}

	secid := strings.ToUpper(strings.TrimSpace(req.GetSecid()))
	board := strings.ToUpper(strings.TrimSpace(req.GetBoard()))
	if secid == "" {
		return nil, status.Error(codes.InvalidArgument, "secid is required")
	}
	tgt := createTarget(req)
	if err := tgt.validate(); err != nil {
		return nil, err
	}

	in, err := s.loadInstrument(ctx, secid, board)
	if err != nil {
		return nil, err
	}
	base, targetPrice, direction, err := tgt.resolve(in)
	if err != nil {
		return nil, err
	}

	n, err := s.Store.CountAlerts(ctx, userID)
	if err != nil {
		s.Log.Error("count alerts", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	if n >= s.MaxAlertsPerUser {
		return nil, status.Errorf(codes.FailedPrecondition, "too many alerts (limit %d)", s.MaxAlertsPerUser)
	}

	a, err := s.Store.CreateAlert(ctx, storage.Alert{
		UserID: userID, SecID: in.SecID, Board: in.Board, Direction: direction,
		BasePrice: base, TargetPrice: targetPrice, InputPercent: tgt.percent,
	})
	if err != nil {
		s.Log.Error("create alert", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	a.ShortName, a.Currency, a.PriceInPercent = in.ShortName, in.Currency, in.PriceInPercent
	a.CurrentPrice = in.LastPrice
	return toAlertPB(a), nil
}

func (s *Server) UpdateAlert(ctx context.Context, req *notifierpb.UpdateAlertRequest) (*notifierpb.Alert, error) {
	userID, err := authmd.UserID(ctx)
	if err != nil {
		return nil, err
	}
	tgt := updateTarget(req)
	if err := tgt.validate(); err != nil {
		return nil, err
	}

	old, err := s.Store.GetAlert(ctx, userID, req.GetId())
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return nil, status.Error(codes.NotFound, "alert not found")
	case err != nil:
		s.Log.Error("get alert", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	secid, board := old.SecID, old.Board
	if v := strings.ToUpper(strings.TrimSpace(req.GetSecid())); v != "" {
		secid, board = v, strings.ToUpper(strings.TrimSpace(req.GetBoard()))
	}
	in, err := s.loadInstrument(ctx, secid, board)
	if err != nil {
		return nil, err
	}
	base, targetPrice, direction, err := tgt.resolve(in)
	if err != nil {
		return nil, err
	}

	old.SecID, old.Board = in.SecID, in.Board
	old.Direction, old.BasePrice, old.TargetPrice = direction, base, targetPrice
	old.InputPercent = tgt.percent
	a, err := s.Store.UpdateAlert(ctx, old)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return nil, status.Error(codes.NotFound, "alert not found")
	case err != nil:
		s.Log.Error("update alert", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	a.ShortName, a.Currency, a.PriceInPercent = in.ShortName, in.Currency, in.PriceInPercent
	a.CurrentPrice = in.LastPrice
	return toAlertPB(a), nil
}

func (s *Server) loadInstrument(ctx context.Context, secid, board string) (storage.Instrument, error) {
	in, err := s.Store.Instrument(ctx, secid, board)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return storage.Instrument{}, status.Error(codes.NotFound, "unknown security")
	case errors.Is(err, storage.ErrAmbiguousBoard):
		return storage.Instrument{}, status.Error(codes.InvalidArgument, "security trades on several boards, board is required")
	case err != nil:
		s.Log.Error("load instrument", "error", err)
		return storage.Instrument{}, status.Error(codes.Internal, "internal error")
	}
	return in, nil
}

func (s *Server) ListAlerts(ctx context.Context, _ *emptypb.Empty) (*notifierpb.ListAlertsResponse, error) {
	userID, err := authmd.UserID(ctx)
	if err != nil {
		return nil, err
	}
	list, err := s.Store.ListAlerts(ctx, userID)
	if err != nil {
		s.Log.Error("list alerts", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	out := make([]*notifierpb.Alert, 0, len(list))
	for _, a := range list {
		out = append(out, toAlertPB(a))
	}
	return &notifierpb.ListAlertsResponse{Alerts: out}, nil
}

func (s *Server) DeleteAlert(ctx context.Context, req *notifierpb.DeleteAlertRequest) (*emptypb.Empty, error) {
	userID, err := authmd.UserID(ctx)
	if err != nil {
		return nil, err
	}
	err = s.Store.DeleteAlert(ctx, userID, req.GetId())
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return nil, status.Error(codes.NotFound, "alert not found")
	case err != nil:
		s.Log.Error("delete alert", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) CreateTelegramLink(ctx context.Context, _ *emptypb.Empty) (*notifierpb.TelegramLinkToken, error) {
	userID, err := authmd.UserID(ctx)
	if err != nil {
		return nil, err
	}
	if s.Bot == nil {
		return nil, status.Error(codes.FailedPrecondition, "telegram bot is not configured")
	}
	username, ok := s.Bot.Username()
	if !ok {
		return nil, status.Error(codes.FailedPrecondition, "telegram bot is not ready yet")
	}

	token, hash, err := linktoken.Generate()
	if err != nil {
		s.Log.Error("generate link token", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	expiresAt := s.now().Add(s.LinkTokenTTL)
	if err := s.Store.CreateLinkToken(ctx, userID, hash, expiresAt); err != nil {
		s.Log.Error("store link token", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &notifierpb.TelegramLinkToken{
		Url:       "https://t.me/" + url.PathEscape(username) + "?start=" + token,
		ExpiresAt: timestamppb.New(expiresAt),
	}, nil
}

func (s *Server) GetTelegramLink(ctx context.Context, _ *emptypb.Empty) (*notifierpb.TelegramLink, error) {
	userID, err := authmd.UserID(ctx)
	if err != nil {
		return nil, err
	}
	out := &notifierpb.TelegramLink{BotEnabled: s.Bot != nil}
	l, err := s.Store.GetTelegramLink(ctx, userID)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return out, nil
	case err != nil:
		s.Log.Error("get telegram link", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	out.Linked = true
	out.Username = l.Username
	out.LinkedAt = timestamppb.New(l.LinkedAt)
	return out, nil
}

func (s *Server) DeleteTelegramLink(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	userID, err := authmd.UserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.Store.DeleteTelegramLink(ctx, userID); err != nil {
		s.Log.Error("delete telegram link", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &emptypb.Empty{}, nil
}

func roundTo(v float64, decimals *int) float64 {
	d := 6
	if decimals != nil && *decimals >= 0 && *decimals <= 10 {
		d = *decimals
	}
	p := math.Pow10(d)
	return math.Round(v*p) / p
}

func toAlertPB(a storage.Alert) *notifierpb.Alert {
	out := &notifierpb.Alert{
		Id: a.ID, Secid: a.SecID, Board: a.Board, ShortName: a.ShortName,
		BasePrice: a.BasePrice, TargetPrice: a.TargetPrice,
		ChangePercent:  math.Round((a.TargetPrice-a.BasePrice)/a.BasePrice*100*100) / 100,
		CurrentPrice:   a.CurrentPrice,
		PriceInPercent: a.PriceInPercent,
		Currency:       a.Currency,
		CreatedAt:      timestamppb.New(a.CreatedAt),
		UpdatedAt:      timestamppb.New(a.UpdatedAt),
		Status:         notifierpb.AlertStatus_ALERT_STATUS_ACTIVE,
		TriggeredPrice: a.TriggeredPrice,
		InputPercent:   a.InputPercent,
	}
	switch a.Direction {
	case storage.DirectionAbove:
		out.Direction = notifierpb.Direction_DIRECTION_ABOVE
	case storage.DirectionBelow:
		out.Direction = notifierpb.Direction_DIRECTION_BELOW
	}
	if a.TriggeredAt != nil {
		out.Status = notifierpb.AlertStatus_ALERT_STATUS_TRIGGERED
		out.TriggeredAt = timestamppb.New(*a.TriggeredAt)
	}
	return out
}
