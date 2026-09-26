package parsing

import (
	"errors"
	"testing"

	"invest/backend/services/parser/internal/task"
)

type staticParser struct {
	trades []Trade
	err    error
}

func (p staticParser) Parse(task.ReportUploaded, []byte) (Report, error) {
	return Report{Trades: p.trades}, p.err
}

func TestDispatcher_ResolvesByBroker(t *testing.T) {
	want := []Trade{{SecID: "SBER", Board: "TQBR", Side: "buy", Quantity: 1, Price: 100}}
	d := NewDispatcher()
	d.Register("tinkoff", staticParser{trades: want})

	got, err := d.Parse(task.ReportUploaded{Broker: "tinkoff"}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got.Trades) != 1 || got.Trades[0].SecID != "SBER" {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestDispatcher_KeyNormalization(t *testing.T) {

	want := []Trade{{SecID: "LKOH", Board: "TQBR", Side: "sell", Quantity: 2, Price: 5000}}
	d := NewDispatcher()
	d.Register("TCS Investments", staticParser{trades: want})

	got, err := d.Parse(task.ReportUploaded{Broker: "tcs investments"}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got.Trades) != 1 || got.Trades[0].SecID != "LKOH" {
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
