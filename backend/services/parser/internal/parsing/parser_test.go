package parsing

import (
	"errors"
	"testing"

	"invest/backend/services/parser/internal/task"
)

// staticParser is a fake Parser that returns canned trades/error.
type staticParser struct {
	trades []Trade
	err    error
}

func (p staticParser) Parse(task.ReportUploaded, []byte) ([]Trade, error) {
	return p.trades, p.err
}

func TestDispatcher_ResolvesByBroker(t *testing.T) {
	want := []Trade{{SecID: "SBER", Board: "TQBR", Side: "buy", Quantity: 1, Price: 100}}
	d := NewDispatcher()
	d.Register("tinkoff", staticParser{trades: want})

	got, err := d.Parse(task.ReportUploaded{Broker: "tinkoff"}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got) != 1 || got[0].SecID != "SBER" {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestDispatcher_KeyNormalization(t *testing.T) {
	// The gateway normalizes user input ("TCS Investments" ->
	// "tcs-investments") before publishing; the dispatcher must apply
	// the same normalization on both sides, so a task published with a
	// differently-cased/spaced-but-equivalent key still resolves.
	want := []Trade{{SecID: "LKOH", Board: "TQBR", Side: "sell", Quantity: 2, Price: 5000}}
	d := NewDispatcher()
	d.Register("TCS Investments", staticParser{trades: want})

	got, err := d.Parse(task.ReportUploaded{Broker: "tcs investments"}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got) != 1 || got[0].SecID != "LKOH" {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestDispatcher_UnknownBroker(t *testing.T) {
	d := NewDispatcher()
	d.Register("tinkoff", staticParser{})

	_, err := d.Parse(task.ReportUploaded{Broker: "sber"}, nil)
	var unsupported *ErrUnsupportedBroker
	if !errors.As(err, &unsupported) {
		t.Fatalf("err = %v, want ErrUnsupportedBroker", err)
	}
	if unsupported.Broker != "sber" {
		t.Errorf("unsupported.Broker = %q, want %q", unsupported.Broker, "sber")
	}
}

func TestDispatcher_EmptyBroker(t *testing.T) {
	d := NewDispatcher()
	d.Register("tinkoff", staticParser{})

	_, err := d.Parse(task.ReportUploaded{}, nil)
	var unsupported *ErrUnsupportedBroker
	if !errors.As(err, &unsupported) {
		t.Fatalf("err = %v, want ErrUnsupportedBroker (empty broker key)", err)
	}
}
