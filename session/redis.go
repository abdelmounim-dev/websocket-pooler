package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

// RedisStore implements the Store interface using Redis.
type RedisStore struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisStore creates a new RedisStore.
func NewRedisStore(client *redis.Client, ttl time.Duration) Store {
	return &RedisStore{
		client: client,
		ttl:    ttl,
	}
}

func sessionKey(clientID string) string {
	return fmt.Sprintf("session:%s", clientID)
}

// Create stores a new session in Redis with a TTL.
func (s *RedisStore) Create(ctx context.Context, session *Session) error {
	key := sessionKey(session.ClientID)
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}
	return s.client.Set(ctx, key, data, s.ttl).Err()
}

// Get retrieves a session from Redis.
func (s *RedisStore) Get(ctx context.Context, clientID string) (*Session, error) {
	key := sessionKey(clientID)
	data, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Not found is not an error, just means no session
		}
		return nil, err
	}

	var session Session
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}
	return &session, nil
}

// Delete removes a session from Redis.
func (s *RedisStore) Delete(ctx context.Context, clientID string) error {
	key := sessionKey(clientID)
	return s.client.Del(ctx, key).Err()
}

// RefreshTTL updates the expiration time of a session key in Redis.
func (s *RedisStore) RefreshTTL(ctx context.Context, clientID string) error {
	key := sessionKey(clientID)
	// Expire returns true if the timeout was set.
	// If the key doesn't exist, it's a no-op which is fine.
	return s.client.Expire(ctx, key, s.ttl).Err()
}
