package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"invest/backend/services/parser/internal/parsing"
	"invest/backend/services/parser/internal/portfolioclient"
	"invest/backend/services/parser/internal/task"
)

func TestIsPermanent(t *testing.T) {
	cases := map[error]bool{
		status.Error(codes.FailedPrecondition, "unknown secid"):                true,
		status.Error(codes.NotFound, "portfolio not found"):                    true,
		status.Error(codes.InvalidArgument, "bad"):                             true,
		status.Error(codes.Unavailable, "down"):                                false,
		status.Error(codes.DeadlineExceeded, "slow"):                           false,
		fmt.Errorf("wrapped: %w", status.Error(codes.FailedPrecondition, "x")): true,
		errors.New("plain"): false,
	}
	for err, want := range cases {
		if got := isPermanent(err); got != want {
			t.Errorf("isPermanent(%v) = %v, want %v", err, got, want)
		}
	}
}

type fakeAck struct{ acked, nacked, requeued bool }

func (f *fakeAck) Ack(uint64, bool) error { f.acked = true; return nil }
func (f *fakeAck) Nack(_ uint64, _ bool, requeue bool) error {
	f.nacked, f.requeued = true, requeue
	return nil
}
func (f *fakeAck) Reject(_ uint64, requeue bool) error {
	f.nacked, f.requeued = true, requeue
	return nil
}

type fakeStore struct{ err error }

func (f fakeStore) Download(context.Context, string, string) ([]byte, error) {
	return []byte("<html></html>"), f.err
}

type fakeParser struct {
	report parsing.Report
	err    error
}

func (f fakeParser) Parse(task.ReportUploaded, []byte) (parsing.Report, error) {
	return f.report, f.err
}

type statusCall struct{ id, status, message string }

type fakePortfolio struct {
	statusErr error
	importErr error

	statuses []statusCall
	importID string
}

func (f *fakePortfolio) ImportReport(_ context.Context, _, _, importID string, _ parsing.Report) (portfolioclient.ImportResult, error) {
	f.importID = importID
	if f.importErr != nil {
		return portfolioclient.ImportResult{}, f.importErr
	}
	return portfolioclient.ImportResult{TradesCreated: 1}, nil
}

func (f *fakePortfolio) SetImportStatus(_ context.Context, _, importID, st, message string) error {
	f.statuses = append(f.statuses, statusCall{importID, st, message})
	if st == statusProcessing && f.statusErr != nil {
		return fmt.Errorf("wrapped: %w", f.statusErr)
	}
	return nil
}

var oneTrade = parsing.Report{Trades: []parsing.Trade{{SecID: "SBER"}}}

func runTask(t *testing.T, store Downloader, parser parsing.Parser, pf *fakePortfolio) *fakeAck {
	t.Helper()
	body, _ := json.Marshal(task.ReportUploaded{TaskID: "task-1", UserID: "u1", PortfolioID: "p1", Broker: "sber"})
	ack := &fakeAck{}
	w := &Worker{Store: store, Parser: parser, Portfolio: pf, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	w.handle(context.Background(), amqp.Delivery{Acknowledger: ack, Body: body})
	return ack
}

func TestHandle_Success(t *testing.T) {
	pf := &fakePortfolio{}
	ack := runTask(t, fakeStore{}, fakeParser{report: oneTrade}, pf)
	if !ack.acked {
		t.Error("not acked")
	}
	if pf.importID != "task-1" {
		t.Errorf("import id = %q, want the task id", pf.importID)
	}
	if len(pf.statuses) != 1 || pf.statuses[0].status != statusProcessing {
		t.Errorf("statuses = %v, want only processing (done is set by ImportReport)", pf.statuses)
	}
}

func TestHandle_EmptyReportIsDone(t *testing.T) {
	pf := &fakePortfolio{}
	ack := runTask(t, fakeStore{}, fakeParser{}, pf)
	if !ack.acked {
		t.Error("not acked")
	}
	if last := pf.statuses[len(pf.statuses)-1]; last.status != statusDone {
		t.Errorf("last status = %v, want done", last)
	}
}

func TestHandle_ParseErrorFails(t *testing.T) {
	pf := &fakePortfolio{}
	ack := runTask(t, fakeStore{}, fakeParser{err: errors.New("no trades table")}, pf)
	if !ack.nacked || ack.requeued {
		t.Errorf("ack = %+v, want dropped", ack)
	}
	if last := pf.statuses[len(pf.statuses)-1]; last.status != statusFailed || last.message != msgParseFailed {
		t.Errorf("last status = %v, want failed with the parse message", last)
	}
}

func TestHandle_UnsupportedBroker(t *testing.T) {
	pf := &fakePortfolio{}
	runTask(t, fakeStore{}, fakeParser{err: &parsing.ErrUnsupportedBroker{Broker: "x"}}, pf)
	if last := pf.statuses[len(pf.statuses)-1]; last.message != msgUnsupportedBroker {
		t.Errorf("last status = %v, want unsupported broker message", last)
	}
}

func TestHandle_UnknownInstrument(t *testing.T) {
	pf := &fakePortfolio{importErr: fmt.Errorf("import report: %w", status.Error(codes.FailedPrecondition,
		"unknown secid/board (price_updater hasn't seen this instrument): unknown instrument: no such secid/board in securities: ABCD/TQBR"))}
	ack := runTask(t, fakeStore{}, fakeParser{report: oneTrade}, pf)
	if !ack.nacked || ack.requeued {
		t.Errorf("ack = %+v, want dropped", ack)
	}
	want := msgUnknownInstrument + ": ABCD/TQBR"
	if last := pf.statuses[len(pf.statuses)-1]; last.status != statusFailed || last.message != want {
		t.Errorf("last status = %v, want failed %q", last, want)
	}
}

func TestHandle_TransientErrorsRequeueWithoutFailing(t *testing.T) {
	pf := &fakePortfolio{}
	ack := runTask(t, fakeStore{err: errors.New("minio down")}, fakeParser{report: oneTrade}, pf)
	if !ack.requeued {
		t.Errorf("download error: ack = %+v, want requeue", ack)
	}

	pf = &fakePortfolio{importErr: status.Error(codes.Unavailable, "down")}
	ack = runTask(t, fakeStore{}, fakeParser{report: oneTrade}, pf)
	if !ack.requeued {
		t.Errorf("portfolio down: ack = %+v, want requeue", ack)
	}
	for _, s := range pf.statuses {
		if s.status == statusFailed {
			t.Errorf("marked failed on a transient error: %v", pf.statuses)
		}
	}
}

func TestHandle_NoImportRowIsNotTracked(t *testing.T) {
	pf := &fakePortfolio{statusErr: status.Error(codes.NotFound, "report import not found")}
	ack := runTask(t, fakeStore{}, fakeParser{err: errors.New("bad")}, pf)
	if !ack.nacked {
		t.Error("not nacked")
	}
	if len(pf.statuses) != 1 {
		t.Errorf("statuses = %v, want only the processing attempt", pf.statuses)
	}
}
