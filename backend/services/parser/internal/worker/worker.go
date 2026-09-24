package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"invest/backend/services/parser/internal/objectstore"
	"invest/backend/services/parser/internal/parsing"
	"invest/backend/services/parser/internal/portfolioclient"
	"invest/backend/services/parser/internal/task"
)

type Worker struct {
	Store     *objectstore.Store
	Parser    parsing.Parser
	Portfolio *portfolioclient.Client
	Log       *slog.Logger
}

func (w *Worker) Run(ctx context.Context, deliveries <-chan amqp.Delivery) {
	for {
		select {
		case <-ctx.Done():
			return
		case d, ok := <-deliveries:
			if !ok {
				return
			}
			w.handle(ctx, d)
		}
	}
}

func (w *Worker) handle(ctx context.Context, d amqp.Delivery) {
	var t task.ReportUploaded
	if err := json.Unmarshal(d.Body, &t); err != nil {
		w.Log.Error("invalid task payload, dropping", "error", err)
		_ = d.Nack(false, false)
		return
	}
	log := w.Log.With("task_id", t.TaskID, "user_id", t.UserID, "portfolio_id", t.PortfolioID, "broker", t.Broker)

	data, err := w.Store.Download(ctx, t.Bucket, t.ObjectKey)
	if err != nil {
		log.Error("download report from minio, requeueing", "error", err)
		_ = d.Nack(false, true)
		return
	}

	report, err := w.Parser.Parse(t, data)
	if err != nil {
		var unsupported *parsing.ErrUnsupportedBroker
		switch {
		case errors.As(err, &unsupported):
			log.Error("unsupported broker, dropping task", "error", err)
		case errors.Is(err, parsing.ErrNotImplemented):
			log.Warn("parsing not implemented yet, dropping task", "error", err)
		default:
			log.Error("parse report, dropping task", "error", err)
		}

		_ = d.Nack(false, false)
		return
	}

	if report.Empty() {
		
		
		log.Info("report has no trades or cash operations, nothing to import")
		_ = d.Ack(false)
		return
	}

	res, err := w.Portfolio.ImportReport(ctx, t.UserID, t.PortfolioID, report)
	if err != nil {
		if isPermanent(err) {
			
			
			
			log.Error("portfolio rejected report, dropping task", "error", err)
			_ = d.Nack(false, false)
			return
		}
		log.Error("import report into portfolio, requeueing", "error", err)
		_ = d.Nack(false, true)
		return
	}

	log.Info("task processed",
		"trades_created", res.TradesCreated, "trades_skipped", res.TradesSkipped,
		"cash_created", res.CashCreated, "cash_skipped", res.CashSkipped)
	_ = d.Ack(false)
}





func isPermanent(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	switch st.Code() {
	case codes.InvalidArgument, codes.FailedPrecondition, codes.NotFound,
		codes.AlreadyExists, codes.PermissionDenied, codes.Unauthenticated:
		return true
	}
	return false
}
