package grpcserver

import (
	"context"
	"errors"
	"path"
	"regexp"
	"slices"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"invest/backend/services/portfolio/internal/authmd"
	"invest/backend/services/portfolio/internal/storage"
	portfoliopb "invest/backend/services/portfolio/proto"
)

func (s *Server) ListBrokers(ctx context.Context, _ *emptypb.Empty) (*portfoliopb.ListBrokersResponse, error) {
	if _, err := authmd.UserID(ctx); err != nil {
		return nil, err
	}
	brokers, err := s.Store.ListBrokers(ctx)
	if err != nil {
		s.Log.Error("list brokers", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	out := make([]*portfoliopb.Broker, 0, len(brokers))
	for _, b := range brokers {
		out = append(out, &portfoliopb.Broker{
			Id:          b.ID,
			Name:        b.Name,
			FileFormats: b.FileFormats,
			IconUrl:     b.IconURL,
			Color:       b.Color,
		})
	}
	return &portfoliopb.ListBrokersResponse{Brokers: out}, nil
}

func (s *Server) CreateReportImport(ctx context.Context, req *portfoliopb.CreateReportImportRequest) (*portfoliopb.ReportImport, error) {
	id := strings.TrimSpace(req.GetId())
	if !uuidRE.MatchString(id) {
		return nil, status.Error(codes.InvalidArgument, "id must be a UUID")
	}

	p, err := s.loadOwnedPortfolio(ctx, req.GetPortfolioId())
	if err != nil {
		return nil, err
	}

	brokerID := strings.ToLower(strings.TrimSpace(req.GetBrokerId()))
	broker, err := s.Store.GetBroker(ctx, brokerID)
	if errors.Is(err, storage.ErrNotFound) || (err == nil && !broker.Enabled) {
		return nil, status.Error(codes.InvalidArgument, "Отчёты этого брокера пока не поддерживаются")
	}
	if err != nil {
		s.Log.Error("get broker", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	filename := strings.TrimSpace(req.GetFilename())
	if !acceptsFile(broker.FileFormats, filename) {
		return nil, status.Errorf(codes.InvalidArgument, "Неподдерживаемый формат файла. Нужен %s", formatList(broker.FileFormats))
	}

	r, err := s.Store.CreateReportImport(ctx, storage.ReportImport{
		ID:          id,
		PortfolioID: p.ID,
		BrokerID:    broker.ID,
		Filename:    filename,
	})
	if errors.Is(err, storage.ErrDuplicate) {
		return nil, status.Error(codes.AlreadyExists, "report import already exists")
	}
	if err != nil {
		s.Log.Error("create report import", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	return toReportImportPB(r), nil
}

func (s *Server) GetReportImport(ctx context.Context, req *portfoliopb.GetReportImportRequest) (*portfoliopb.ReportImport, error) {
	r, err := s.loadOwnedReportImport(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return toReportImportPB(r), nil
}

func (s *Server) ListReportImports(ctx context.Context, req *portfoliopb.ListReportImportsRequest) (*portfoliopb.ListReportImportsResponse, error) {
	p, err := s.loadOwnedPortfolio(ctx, req.GetPortfolioId())
	if err != nil {
		return nil, err
	}
	list, err := s.Store.ListReportImports(ctx, p.ID)
	if err != nil {
		s.Log.Error("list report imports", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	out := make([]*portfoliopb.ReportImport, 0, len(list))
	for _, r := range list {
		out = append(out, toReportImportPB(r))
	}
	return &portfoliopb.ListReportImportsResponse{Imports: out}, nil
}

var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const maxImportErrorLen = 1000

func (s *Server) UpdateReportImportStatus(ctx context.Context, req *portfoliopb.UpdateReportImportStatusRequest) (*portfoliopb.ReportImport, error) {
	st := strings.ToLower(strings.TrimSpace(req.GetStatus()))
	switch st {
	case storage.ImportProcessing, storage.ImportFailed, storage.ImportDone:
	default:
		return nil, status.Error(codes.InvalidArgument, `status must be "processing", "failed" or "done"`)
	}
	errMsg := ""
	if st == storage.ImportFailed {
		errMsg = strings.TrimSpace(req.GetError())
		if errMsg == "" {
			errMsg = "Не удалось обработать отчёт"
		}
		if r := []rune(errMsg); len(r) > maxImportErrorLen {
			errMsg = string(r[:maxImportErrorLen])
		}
	}

	if _, err := s.loadOwnedReportImport(ctx, req.GetId()); err != nil {
		return nil, err
	}

	r, err := s.Store.UpdateReportImportStatus(ctx, req.GetId(), st, errMsg)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return nil, status.Error(codes.NotFound, "report import not found")
	case errors.Is(err, storage.ErrImportFinished):
		return nil, status.Error(codes.FailedPrecondition, "report import is already finished")
	case err != nil:
		s.Log.Error("update report import status", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	return toReportImportPB(r), nil
}

func (s *Server) loadOwnedReportImport(ctx context.Context, id string) (storage.ReportImport, error) {
	if _, err := authmd.UserID(ctx); err != nil {
		return storage.ReportImport{}, err
	}
	r, err := s.Store.GetReportImport(ctx, id)
	if errors.Is(err, storage.ErrNotFound) {
		return storage.ReportImport{}, status.Error(codes.NotFound, "report import not found")
	}
	if err != nil {
		s.Log.Error("get report import", "error", err)
		return storage.ReportImport{}, status.Error(codes.Internal, "internal error")
	}
	if _, err := s.loadOwnedPortfolio(ctx, r.PortfolioID); err != nil {
		if status.Code(err) == codes.NotFound {
			return storage.ReportImport{}, status.Error(codes.NotFound, "report import not found")
		}
		return storage.ReportImport{}, err
	}
	return r, nil
}

func acceptsFile(formats []string, filename string) bool {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(filename), "."))
	if ext == "" {
		return false
	}
	if ext == "htm" {
		ext = "html"
	}
	return slices.Contains(formats, ext)
}

func formatList(formats []string) string {
	up := make([]string, 0, len(formats))
	for _, f := range formats {
		up = append(up, strings.ToUpper(f))
	}
	return strings.Join(up, ", ")
}

func toReportImportPB(r storage.ReportImport) *portfoliopb.ReportImport {
	out := &portfoliopb.ReportImport{
		Id:                    r.ID,
		PortfolioId:           r.PortfolioID,
		BrokerId:              r.BrokerID,
		Filename:              r.Filename,
		Status:                r.Status,
		Error:                 r.Error,
		TradesCreated:         int32(r.TradesCreated),
		TradesSkipped:         int32(r.TradesSkipped),
		CashOperationsCreated: int32(r.CashCreated),
		CashOperationsSkipped: int32(r.CashSkipped),
		CreatedAt:             timestamppb.New(r.CreatedAt),
		UpdatedAt:             timestamppb.New(r.UpdatedAt),
	}
	if r.FinishedAt != nil {
		out.FinishedAt = timestamppb.New(*r.FinishedAt)
	}
	return out
}
