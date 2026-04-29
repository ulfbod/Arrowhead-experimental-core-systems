// Package broker provides a reusable publish/subscribe abstraction over RabbitMQ
// using a topic exchange. Designed for use by Arrowhead experiment services.
//
// All experiment services that need messaging import this package and wire it
// with a concrete AMQP URL at startup. Core systems must NOT import this package.
package broker

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Config holds connection and exchange configuration.
type Config struct {
	// URL is the AMQP connection string, e.g. amqp://guest:guest@localhost:5672/
	URL string
	// Exchange is the topic exchange name. Defaults to "arrowhead" if empty.
	Exchange string
}

// Handler is called once per received message.
type Handler func(payload []byte)

// Broker manages a single RabbitMQ connection and channel.
// It declares a durable topic exchange on construction and is not
// safe for concurrent Publish calls from multiple goroutines without
// external synchronisation.
type Broker struct {
	conn     *amqp.Connection
	ch       *amqp.Channel
	exchange string
}

// New dials RabbitMQ and declares the topic exchange.
// Returns an error if the connection or channel cannot be established.
func New(cfg Config) (*Broker, error) {
	exchange := cfg.Exchange
	if exchange == "" {
		exchange = "arrowhead"
	}

	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("broker: dial %s: %w", cfg.URL, err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("broker: open channel: %w", err)
	}

	if err := ch.ExchangeDeclare(
		exchange, // name
		"topic",  // kind
		true,     // durable
		false,    // auto-delete
		false,    // internal
		false,    // no-wait
		nil,      // args
	); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("broker: declare exchange %q: %w", exchange, err)
	}

	return &Broker{conn: conn, ch: ch, exchange: exchange}, nil
}

// Publish sends payload to the given routing key on the topic exchange.
// The message is marked persistent (DeliveryMode 2).
func (b *Broker) Publish(routingKey string, payload []byte) error {
	return b.ch.Publish(
		b.exchange,  // exchange
		routingKey,  // routing key
		false,       // mandatory
		false,       // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         payload,
		},
	)
}

// Subscribe declares a durable queue, binds it to the exchange with bindingKey,
// and starts delivering messages to handler in a background goroutine.
// Multiple calls with the same queue name share the same queue.
// The handler receives the raw message body; acknowledgement is automatic.
func (b *Broker) Subscribe(queue, bindingKey string, handler Handler) error {
	q, err := b.ch.QueueDeclare(
		queue, // name
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("broker: declare queue %q: %w", queue, err)
	}

	if err := b.ch.QueueBind(q.Name, bindingKey, b.exchange, false, nil); err != nil {
		return fmt.Errorf("broker: bind queue %q key %q: %w", queue, bindingKey, err)
	}

	msgs, err := b.ch.Consume(
		q.Name, // queue
		"",     // consumer tag (auto)
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		return fmt.Errorf("broker: consume queue %q: %w", queue, err)
	}

	go func() {
		for d := range msgs {
			handler(d.Body)
		}
	}()

	return nil
}

// Close releases the AMQP channel and connection.
func (b *Broker) Close() error {
	if err := b.ch.Close(); err != nil {
		return err
	}
	return b.conn.Close()
}
