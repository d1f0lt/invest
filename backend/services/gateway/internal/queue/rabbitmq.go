package queue

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"invest/backend/services/gateway/internal/task"
)

type Publisher struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	queue   string
}

func Connect(url, queueName string) (*Publisher, error) {
	conn, err := amqp.DialConfig(url, amqp.Config{Recovery: &amqp.Recovery{}})
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}

	if _, err := ch.QueueDeclare(queueName, true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("declare queue %q: %w", queueName, err)
	}

	return &Publisher{conn: conn, channel: ch, queue: queueName}, nil
}

func (p *Publisher) Publish(ctx context.Context, t task.ReportUploaded) error {
	body, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}

	err = p.channel.PublishWithContext(ctx, "", p.queue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
	if err != nil {
		return fmt.Errorf("publish to queue %q: %w", p.queue, err)
	}
	return nil
}

func (p *Publisher) Ping(_ context.Context) error {
	if p.conn == nil || p.conn.IsClosed() {
		return fmt.Errorf("rabbitmq connection is closed")
	}
	return nil
}

func (p *Publisher) Close() error {
	var err error
	if p.channel != nil {
		err = p.channel.Close()
	}
	if p.conn != nil {
		if cerr := p.conn.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}
	return err
}
