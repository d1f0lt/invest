package parsing

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"invest/backend/services/parser/internal/task"
)

type Trade struct {
	SecID    string
	Board    string
	Side     string
	Quantity float64
	Price    float64
	Fee      float64
	Currency string

	ExecutedAt *time.Time
}

var ErrNotImplemented = errors.New("parsing: not implemented")

// ErrUnsupportedBroker means no parser is registered for the task's
// broker key. Deliberately not retryable: requeueing the task won't
// make an unknown broker known, so the worker drops it (same handling
// as other parse errors, vs. requeueable infrastructure errors).
type ErrUnsupportedBroker struct{ Broker string }

func (e *ErrUnsupportedBroker) Error() string {
	return fmt.Sprintf("parsing: no parser registered for broker %q", e.Broker)
}

type Parser interface {
	Parse(t task.ReportUploaded, data []byte) ([]Trade, error)
}

// Dispatcher selects the concrete parser by the task's broker key -
// the map[broker]Parser registry this service is built around. The
// zero map value plus Register lets cmd/parser assemble the registry
// without a constructor, the same way other services wire their
// dependencies.
type Dispatcher struct {
	parsers map[string]Parser
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{parsers: map[string]Parser{}}
}

// Register adds p under broker. Keys are normalized (lowercase, spaces
// -> "-") to match the gateway's normalizeBroker, so a task published
// with "TCS" or "tcs" both resolve to the same parser.
func (d *Dispatcher) Register(broker string, p Parser) {
	d.parsers[normalizeBroker(broker)] = p
}

func (d *Dispatcher) Parse(t task.ReportUploaded, data []byte) ([]Trade, error) {
	p, ok := d.parsers[normalizeBroker(t.Broker)]
	if !ok {
		return nil, &ErrUnsupportedBroker{Broker: t.Broker}
	}
	return p.Parse(t, data)
}

// SupportedBrokers lists the registered keys, for startup logging.
func (d *Dispatcher) SupportedBrokers() []string {
	out := make([]string, 0, len(d.parsers))
	for k := range d.parsers {
		out = append(out, k)
	}
	return out
}

func normalizeBroker(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(name))), "-")
}

type Stub struct{}

func (Stub) Parse(task.ReportUploaded, []byte) ([]Trade, error) {
	return nil, ErrNotImplemented
}
