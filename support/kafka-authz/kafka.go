package main

import (
	"context"
	"fmt"
	"time"

	kafka "github.com/segmentio/kafka-go"
)

// KafkaReader wraps a segmentio kafka-go Reader with a typed interface.
type KafkaReader struct {
	r *kafka.Reader
}

// newKafkaReader creates a partition-level reader for the given topic.
//
// We intentionally do NOT use a consumer group here.  SSE streaming is
// inherently real-time: each connection needs its own independent cursor
// starting at the latest offset.  Consumer groups add session-timeout
// overhead, rebalancing delays (especially when the previous kafka-authz
// instance was killed and its member hasn't timed out yet), and committed-
// offset bookkeeping that is irrelevant for a live-stream use case.
func newKafkaReader(brokers []string, topic string) *KafkaReader {
	return &KafkaReader{
		r: kafka.NewReader(kafka.ReaderConfig{
			Brokers:     brokers,
			Topic:       topic,
			Partition:   0,
			MinBytes:    1,
			MaxBytes:    1 << 20, // 1 MB
			MaxWait:     500 * time.Millisecond,
			StartOffset: kafka.LastOffset,
		}),
	}
}

// ReadMessage reads the next message from Kafka, blocking until one arrives
// or the context is cancelled.
func (kr *KafkaReader) ReadMessage(ctx context.Context) ([]byte, error) {
	msg, err := kr.r.ReadMessage(ctx)
	if err != nil {
		return nil, err
	}
	return msg.Value, nil
}

// Close releases the reader.
func (kr *KafkaReader) Close() error {
	return kr.r.Close()
}

// KafkaWriter publishes messages to a Kafka topic.
type KafkaWriter struct {
	w *kafka.Writer
}

// newKafkaWriter creates a producer for the given topic.
func newKafkaWriter(brokers []string, topic string) *KafkaWriter {
	return &KafkaWriter{
		w: &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        topic,
			Balancer:     &kafka.LeastBytes{},
			BatchSize:    1,
			BatchTimeout: 10 * time.Millisecond,
		},
	}
}

// WriteMessage publishes a message with the given key and value.
func (kw *KafkaWriter) WriteMessage(ctx context.Context, key, value []byte) error {
	return kw.w.WriteMessages(ctx, kafka.Message{Key: key, Value: value})
}

// Close releases the writer.
func (kw *KafkaWriter) Close() error {
	return kw.w.Close()
}

// topicForService maps an Arrowhead service definition to a Kafka topic name.
func topicForService(service string) string {
	return fmt.Sprintf("arrowhead.%s", service)
}
