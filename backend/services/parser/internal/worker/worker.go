package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"invest/backend/services/parser/internal/parsing"
	"invest/backend/services/parser/internal/portfolioclient"
	"invest/backend/services/parser/internal/task"
)


type Downloader interface {
	Download(ctx context.Context, bucket, key string) ([]byte, error)
}



type Portfolio interface {
	ImportReport(ctx context.Context, userID, portfolioID, importID string, r parsing.Report) (portfolioclient.ImportResult, error)
	SetImportStatus(ctx context.Context, userID, importID, status, message string) error
}

type Worker struct {
	Store     Downloader
	Parser    parsing.Parser
	Portfolio Portfolio
	Log       *slog.Logger
}


const (
	statusProcessing = "processing"
	statusDone       = "done"
	statusFailed     = "failed"
)


const (
	msgUnsupportedBroker = "Отчёты этого брокера пока не поддерживаются"
	msgParseFailed       = "Не удалось разобрать файл. Проверьте, что это отчёт брокера в формате HTML"
	msgImportRejected    = "Не удалось добавить операции из отчёта в портфель"
	msgUnknownInstrument = "В отчёте есть бумага, которой пока нет в справочнике биржи"
)

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

	
	
	
	importID := t.TaskID
	if err := w.Portfolio.SetImportStatus(ctx, t.UserID, importID, statusProcessing, ""); err != nil {
		switch status.Code(err) {
		case codes.NotFound:
			log.Warn("no report import for this task, status won't be tracked", "error", err)
			importID = ""
		case codes.FailedPrecondition:
			
			
			log.Info("report import already finished, reprocessing anyway")
		default:
			log.Warn("mark report import processing", "error", err)
		}
	}

	data, err := w.Store.Download(ctx, t.Bucket, t.ObjectKey)
	if err != nil {
		log.Error("download report from minio, requeueing", "error", err)
		_ = d.Nack(false, true)
		return
	}

	report, err := w.Parser.Parse(t, data)
	if err != nil {
		var unsupported *parsing.ErrUnsupportedBroker
		msg := msgParseFailed
		switch {
		case errors.As(err, &unsupported):
			log.Error("unsupported broker, dropping task", "error", err)
			msg = msgUnsupportedBroker
		case errors.Is(err, parsing.ErrNotImplemented):
			log.Warn("parsing not implemented yet, dropping task", "error", err)
			msg = msgUnsupportedBroker
		default:
			log.Error("parse report, dropping task", "error", err)
		}
		w.fail(ctx, log, t.UserID, importID, msg)
		_ = d.Nack(false, false)
		return
	}

	if report.Empty() {
		
		
		log.Info("report has no trades or cash operations, nothing to import")
		w.setStatus(ctx, log, t.UserID, importID, statusDone, "")
		_ = d.Ack(false)
		return
	}

	res, err := w.Portfolio.ImportReport(ctx, t.UserID, t.PortfolioID, importID, report)
	if err != nil {
		if isPermanent(err) {
			
			
			log.Error("portfolio rejected report, dropping task", "error", err)
			w.fail(ctx, log, t.UserID, importID, rejectMessage(err))
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

func (w *Worker) fail(ctx context.Context, log *slog.Logger, userID, importID, message string) {
	w.setStatus(ctx, log, userID, importID, statusFailed, message)
}


func (w *Worker) setStatus(ctx context.Context, log *slog.Logger, userID, importID, st, message string) {
	if importID == "" {
		return
	}
	if err := w.Portfolio.SetImportStatus(ctx, userID, importID, st, message); err != nil {
		log.Error("update report import status", "status", st, "error", err)
	}
}


func rejectMessage(err error) string {
	st, _ := status.FromError(err)
	if st.Code() == codes.FailedPrecondition && strings.Contains(st.Message(), "unknown secid/board") {
		
		msg := st.Message()
		if i := strings.LastIndex(msg, ": "); i >= 0 && i+2 < len(msg) {
			return msgUnknownInstrument + ": " + msg[i+2:]
		}
		return msgUnknownInstrument
	}
	return msgImportRejected
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
