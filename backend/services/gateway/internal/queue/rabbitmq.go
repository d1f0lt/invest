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

const confirmTimeout = 10 * time.Second

type Publisher struct {
	url   string
	queue string

	mu      sync.Mutex
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

func (p *Publisher) ensureLocked() error {
	if p.closed {
		return errors.New("publisher is closed")
	}
	if p.conn != nil && !p.conn.IsClosed() && p.channel != nil && !p.channel.IsClosed() {
		return nil
	}
	return p.connectLocked()
}

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

		p.resetLocked()
		return fmt.Errorf("wait for publisher confirm: %w", err)
	}
	if !acked {
		return fmt.Errorf("rabbitmq nacked message for queue %q", p.queue)
	}
	return nil
}

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
