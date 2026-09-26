package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	reconnectMinDelay = time.Second
	reconnectMaxDelay = 30 * time.Second
)

type Consumer struct {
	url   string
	queue string
	log   *slog.Logger

	mu      sync.RWMutex
	conn    *amqp.Connection
	channel *amqp.Channel
	closed  bool
}

func Connect(url, queueName string, log *slog.Logger) (*Consumer, error) {
	c := &Consumer{url: url, queue: queueName, log: log}
	if err := c.connect(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Consumer) connect() error {
	conn, err := amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("dial rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("open channel: %w", err)
	}

	if _, err := ch.QueueDeclare(c.queue, true, false, false, false, nil); err != nil {
		_ = conn.Close()
		return fmt.Errorf("declare queue %q: %w", c.queue, err)
	}

	if err := ch.Qos(1, 0, false); err != nil {
		_ = conn.Close()
		return fmt.Errorf("set qos: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		_ = conn.Close()
		return errors.New("consumer is closed")
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn, c.channel = conn, ch
	return nil
}

func (c *Consumer) Run(ctx context.Context, consumerTag string, handle func(context.Context, <-chan amqp.Delivery)) {
	delay := reconnectMinDelay
	for {
		c.mu.RLock()
		ch := c.channel
		c.mu.RUnlock()

		var err error
		if ch == nil || ch.IsClosed() {
			err = c.connect()
			if err == nil {
				c.mu.RLock()
				ch = c.channel
				c.mu.RUnlock()
			}
		}

		var deliveries <-chan amqp.Delivery
		if err == nil {
			deliveries, err = ch.Consume(c.queue, consumerTag, false, false, false, false, nil)
			if err != nil {
				err = fmt.Errorf("consume queue %q: %w", c.queue, err)
			}
		}

		if err == nil {
			delay = reconnectMinDelay
			handle(ctx, deliveries)
			if ctx.Err() != nil {
				return
			}
			c.log.Warn("rabbitmq deliveries channel closed, reconnecting")
			c.drop(ch)
			continue
		}

		c.log.Error("rabbitmq consumer setup failed, retrying", "error", err, "retry_in", delay.String())
		c.drop(ch)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, reconnectMaxDelay)
	}
}

func (c *Consumer) drop(ch *amqp.Channel) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.channel != ch || c.conn == nil {
		return
	}
	_ = c.conn.Close()
	c.conn, c.channel = nil, nil
}

func (c *Consumer) Ping(_ context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.conn == nil || c.conn.IsClosed() || c.channel == nil || c.channel.IsClosed() {
		return fmt.Errorf("rabbitmq connection is closed")
	}
	return nil
}

func (c *Consumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	var err error
	if c.conn != nil {
		err = c.conn.Close()
	}
	c.conn, c.channel = nil, nil
	return err
}
