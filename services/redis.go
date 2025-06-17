package services

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

func NewRedisClient(address string, password string, db int, poolSize int, poolTimeout int) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:        address,
		DB:          db,
		Password:    password,
		PoolSize:    poolSize,
		PoolTimeout: time.Duration(poolTimeout) * time.Second,
	})
	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return client, nil
}

func CloseRedisClient(client *redis.Client) error {
	if client == nil {
		return nil
	}
	return client.Close()
}
