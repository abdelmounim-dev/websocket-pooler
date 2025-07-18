package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"github.com/abdelmounim-dev/websocket-pooler/metrics"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-redis/redis/v8"
)

const (
	maxRetries     = 3
	initialBackoff = 100 * time.Millisecond
	maxBackoff     = 5 * time.Second
)

// RedisBroker implements MessageBroker using Redis pub/sub
type RedisBroker struct {
	client *redis.Client
}

// NewRedisBroker creates a new Redis message broker
func NewRedisBroker(client *redis.Client) MessageBroker {
	return &RedisBroker{
		client: client,
	}
}

// Type returns the broker type.
func (b *RedisBroker) Type() string {
	return "redis"
}

// MarshalBinary implements encoding.BinaryMarshaler interface
func (m Message) MarshalBinary() ([]byte, error) {
	return json.Marshal(m)
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler interface
func (m *Message) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, m)
}

// Publish sends a message to the specified channel with retry capability
func (b *RedisBroker) Publish(ctx context.Context, channel string, message Message) error {
	operation := func() error {
		startTime := time.Now()
		err := b.client.Publish(ctx, channel, message).Err()
		duration := time.Since(startTime)

		// Log if the operation is slower than a certain threshold
		if duration > 100*time.Millisecond {
			log.Printf("Slow Redis publish for client %s to channel %s took %s", message.ClientID, channel, duration)
		}

		return err
	}

	backoffStrategy := backoff.WithContext(
		backoff.WithMaxRetries(
			backoff.NewExponentialBackOff(
				backoff.WithInitialInterval(initialBackoff),
				backoff.WithMaxInterval(maxBackoff),
			),
			maxRetries,
		),
		ctx,
	)

	return backoff.RetryNotify(operation, backoffStrategy, func(err error, d time.Duration) {
		metrics.BrokerPublishRetries.WithLabelValues(b.Type()).Inc()
		log.Printf("Retrying Redis publish for %s: %v (next attempt in %s)", message.ClientID, err, d)
	})
}

// Subscribe starts listening for messages on the specified channel
func (b *RedisBroker) Subscribe(ctx context.Context, channel string) (<-chan Message, error) {
	pubsub := b.client.Subscribe(ctx, channel)

	// Test subscription
	_, err := pubsub.Receive(ctx)
	if err != nil {
		pubsub.Close()
		return nil, fmt.Errorf("failed to subscribe to %s: %w", channel, err)
	}

	messages := make(chan Message)

	go func() {
		defer pubsub.Close()
		defer close(messages)

		msgChan := pubsub.Channel()

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-msgChan:
				if !ok {
					return
				}

				var message Message
				if err := json.Unmarshal([]byte(msg.Payload), &message); err != nil {
					log.Printf("Message decode error: %v", err)
					continue
				}

				messages <- message
			}
		}
	}()

	return messages, nil
}

// Close cleans up resources
func (b *RedisBroker) Close() error {
	return nil
}
