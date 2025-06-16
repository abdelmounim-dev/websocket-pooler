package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/cenkalti/backoff/v4"
)

const (
	kafkaMaxRetries     = 3
	kafkaInitialBackoff = 100 * time.Millisecond
	kafkaMaxBackoff     = 5 * time.Second
)

// KafkaBroker implements MessageBroker using Apache Kafka
type KafkaBroker struct {
	brokers       []string
	producer      sarama.SyncProducer
	consumerGroup sarama.ConsumerGroup
	config        *sarama.Config
	mu            sync.RWMutex
	closed        bool
}

// NewKafkaBroker creates a new Kafka message broker
func NewKafkaBroker(brokers []string, groupID string) (*KafkaBroker, error) {
	config := sarama.NewConfig()

	// Producer configuration
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = kafkaMaxRetries
	config.Producer.Return.Successes = true
	config.Producer.Compression = sarama.CompressionSnappy
	config.Producer.Flush.Frequency = 500 * time.Millisecond

	// Consumer configuration
	config.Consumer.Return.Errors = true
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	config.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRoundRobin()
	config.Consumer.Group.Session.Timeout = 10 * time.Second
	config.Consumer.Group.Heartbeat.Interval = 3 * time.Second

	// Version configuration
	config.Version = sarama.V4_0_0_0

	// Create producer
	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	// Create consumer group
	consumerGroup, err := sarama.NewConsumerGroup(brokers, groupID, config)
	if err != nil {
		producer.Close()
		return nil, fmt.Errorf("failed to create Kafka consumer group: %w", err)
	}

	return &KafkaBroker{
		brokers:       brokers,
		producer:      producer,
		consumerGroup: consumerGroup,
		config:        config,
	}, nil
}

// Publish sends a message to the specified channel (topic) with retry capability
func (b *KafkaBroker) Publish(ctx context.Context, channel string, message Message) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return fmt.Errorf("broker is closed")
	}
	b.mu.RUnlock()

	// Marshal message to JSON
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Create Kafka message
	kafkaMsg := &sarama.ProducerMessage{
		Topic: channel,
		Key:   sarama.StringEncoder(message.ClientID), // Use ClientID as partition key
		Value: sarama.ByteEncoder(data),
		Headers: []sarama.RecordHeader{
			{
				Key:   []byte("client_id"),
				Value: []byte(message.ClientID),
			},
		},
		Timestamp: time.Now(),
	}

	operation := func() error {
		_, _, err := b.producer.SendMessage(kafkaMsg)
		return err
	}

	backoffStrategy := backoff.WithContext(
		backoff.WithMaxRetries(
			backoff.NewExponentialBackOff(
				backoff.WithInitialInterval(kafkaInitialBackoff),
				backoff.WithMaxInterval(kafkaMaxBackoff),
			),
			kafkaMaxRetries,
		),
		ctx,
	)

	return backoff.RetryNotify(operation, backoffStrategy, func(err error, d time.Duration) {
		log.Printf("Retrying Kafka publish for %s: %v (next attempt in %s)", message.ClientID, err, d)
	})
}

// Subscribe starts listening for messages on the specified channel (topic)
func (b *KafkaBroker) Subscribe(ctx context.Context, channel string) (<-chan Message, error) {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return nil, fmt.Errorf("broker is closed")
	}
	b.mu.RUnlock()

	messages := make(chan Message, 100) // Buffered channel for better performance

	// Create consumer handler
	handler := &consumerGroupHandler{
		messages: messages,
		ready:    make(chan bool),
	}

	// Start consuming in a goroutine
	go func() {
		defer close(messages)

		for {
			select {
			case <-ctx.Done():
				return
			default:
				// Consumer group consume should be called inside an infinite loop
				if err := b.consumerGroup.Consume(ctx, []string{channel}, handler); err != nil {
					log.Printf("Error from consumer group: %v", err)
					return
				}
			}
		}
	}()

	// Handle consumer group errors
	go func() {
		for err := range b.consumerGroup.Errors() {
			log.Printf("Consumer group error: %v", err)
		}
	}()

	// Wait for consumer to be ready
	select {
	case <-handler.ready:
		return messages, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("timeout waiting for consumer to be ready")
	}
}

// Close cleans up resources
func (b *KafkaBroker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	var errs []error

	if err := b.producer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close producer: %w", err))
	}

	if err := b.consumerGroup.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close consumer group: %w", err))
	}

	b.closed = true

	if len(errs) > 0 {
		return fmt.Errorf("errors during close: %v", errs)
	}

	return nil
}

// consumerGroupHandler implements sarama.ConsumerGroupHandler
type consumerGroupHandler struct {
	messages chan<- Message
	ready    chan bool
	once     sync.Once
}

// Setup is run at the beginning of a new session, before ConsumeClaim
func (h *consumerGroupHandler) Setup(sarama.ConsumerGroupSession) error {
	h.once.Do(func() {
		close(h.ready)
	})
	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
func (h *consumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim must start a consumer loop of ConsumerGroupClaim's Messages().
func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case kafkaMsg := <-claim.Messages():
			if kafkaMsg == nil {
				return nil
			}

			var message Message
			if err := json.Unmarshal(kafkaMsg.Value, &message); err != nil {
				log.Printf("Message decode error: %v", err)
				// Mark message as processed even if decode fails to avoid reprocessing
				session.MarkMessage(kafkaMsg, "")
				continue
			}

			select {
			case h.messages <- message:
				// Message sent successfully
			case <-session.Context().Done():
				return nil
			}

			// Mark message as processed
			session.MarkMessage(kafkaMsg, "")

		case <-session.Context().Done():
			return nil
		}
	}
}
