package queue

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	queue   string
}

func Connect(url, queueName string) (*Consumer, error) {
	conn, err := amqp.Dial(url)
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

	if err := ch.Qos(1, 0, false); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("set qos: %w", err)
	}

	return &Consumer{conn: conn, channel: ch, queue: queueName}, nil
}

func (c *Consumer) Consume(consumerTag string) (<-chan amqp.Delivery, error) {
	deliveries, err := c.channel.Consume(c.queue, consumerTag, false, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("consume queue %q: %w", c.queue, err)
	}
	return deliveries, nil
}

func (c *Consumer) Ping(_ context.Context) error {
	if c.conn == nil || c.conn.IsClosed() {
		return fmt.Errorf("rabbitmq connection is closed")
	}
	return nil
}

func (c *Consumer) Close() error {
	var err error
	if c.channel != nil {
		err = c.channel.Close()
	}
	if c.conn != nil {
		if cerr := c.conn.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}
	return err
}
