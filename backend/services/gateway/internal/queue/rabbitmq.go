package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"invest/backend/services/gateway/internal/task"
)

// confirmTimeout bounds how long Publish waits for the broker to confirm a
// message (on top of whatever deadline the caller's context already has).
const confirmTimeout = 10 * time.Second

// Publisher publishes report.uploaded tasks.
//
// Reliability:
//   - the channel is in confirm mode and Publish returns only after the
//     broker has acked the message (durable queue + persistent message +
//     publisher confirm = the task is stored when the client gets 202);
//   - the connection is re-established lazily: after a RabbitMQ restart the
//     next Publish (or health-check Ping) redials, instead of the gateway
//     staying broken until its container is restarted. This is our own
//     reconnect rather than amqp091-go's experimental Config.Recovery.
type Publisher struct {
	url   string
	queue string

	mu      sync.Mutex // guards the fields below and serializes publishes
	conn    *amqp.Connection
	channel *amqp.Channel
	closed  bool
}

func Connect(url, queueName string) (*Publisher, error) {
	p := &Publisher{url: url, queue: queueName}
	if err := p.connectLocked(); err != nil {
		return nil, err
	}
	return p, nil
}

// connectLocked (re)establishes the connection and channel. p.mu must be
// held (or p must not be shared yet).
func (p *Publisher) connectLocked() error {
	p.resetLocked()

	conn, err := amqp.Dial(p.url)
	if err != nil {
		return fmt.Errorf("dial rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("open channel: %w", err)
	}

	if _, err := ch.QueueDeclare(p.queue, true, false, false, false, nil); err != nil {
		_ = conn.Close()
		return fmt.Errorf("declare queue %q: %w", p.queue, err)
	}

	if err := ch.Confirm(false); err != nil {
		_ = conn.Close()
		return fmt.Errorf("enable publisher confirms: %w", err)
	}

	p.conn, p.channel = conn, ch
	return nil
}

// ensureLocked makes sure there is an open channel, redialing if the
// previous connection or channel has been closed. p.mu must be held.
func (p *Publisher) ensureLocked() error {
	if p.closed {
		return errors.New("publisher is closed")
	}
	if p.conn != nil && !p.conn.IsClosed() && p.channel != nil && !p.channel.IsClosed() {
		return nil
	}
	return p.connectLocked()
}

// resetLocked drops the current connection, if any. p.mu must be held.
func (p *Publisher) resetLocked() {
	if p.conn != nil {
		_ = p.conn.Close()
	}
	p.conn, p.channel = nil, nil
}

func (p *Publisher) Publish(ctx context.Context, t task.ReportUploaded) error {
	body, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, confirmTimeout)
	defer cancel()

	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.ensureLocked(); err != nil {
		return err
	}

	dc, err := p.channel.PublishWithDeferredConfirmWithContext(ctx, "", p.queue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
	if err != nil {
		p.resetLocked()
		return fmt.Errorf("publish to queue %q: %w", p.queue, err)
	}

	acked, err := dc.WaitContext(ctx)
	if err != nil {
		// No confirm in time: the channel's state is unknown, so start
		// from a fresh connection next time.
		p.resetLocked()
		return fmt.Errorf("wait for publisher confirm: %w", err)
	}
	if !acked {
		return fmt.Errorf("rabbitmq nacked message for queue %q", p.queue)
	}
	return nil
}

// Ping reports whether RabbitMQ is reachable, reconnecting if needed so a
// broker restart heals itself on the next health check.
func (p *Publisher) Ping(_ context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.ensureLocked(); err != nil {
		return fmt.Errorf("rabbitmq unavailable: %w", err)
	}
	return nil
}

func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	var err error
	if p.conn != nil {
		err = p.conn.Close()
	}
	p.conn, p.channel = nil, nil
	return err
}
