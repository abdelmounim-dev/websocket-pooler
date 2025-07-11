package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/abdelmounim-dev/websocket-pooler/broker"
	"github.com/abdelmounim-dev/websocket-pooler/config"
	"github.com/abdelmounim-dev/websocket-pooler/metrics"
	"github.com/abdelmounim-dev/websocket-pooler/server"
	"github.com/abdelmounim-dev/websocket-pooler/session"
	"github.com/abdelmounim-dev/websocket-pooler/websocket"
	"github.com/cenkalti/backoff/v4"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

func main() {
	// Initialize context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize config
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = "dev"
	}
	if err := config.Initialize(env); err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}
	cfg := config.Get()

	// Initialize and start metrics server
	if cfg.Metrics.Enabled {
		metrics.StartServer(cfg.Metrics.Port, cfg.Metrics.Path)
	}

	// Generate a unique ID for this server instance
	serverID := uuid.New().String()
	log.Printf("Starting server instance with ID: %s", serverID)

	// Session Store always uses Redis in this architecture.
	// Create the Redis client for the session store.
	var redisClient *redis.Client

	// Define the operation to connect to Redis
	connectToRedis := func() error {
		log.Printf("Attempting to connect to Redis at %s...", cfg.Broker.Redis.Address)
		client := redis.NewClient(&redis.Options{
			Addr:     cfg.Broker.Redis.Address,
			Password: cfg.Broker.Redis.Password,
			DB:       cfg.Broker.Redis.DB,
			PoolSize: cfg.Broker.Redis.PoolSize,
		})

		// Ping to check the connection
		if err := client.Ping(context.Background()).Err(); err != nil {
			log.Printf("Failed to ping Redis: %v. Retrying...", err)
			return err // Important: return the error to trigger a retry
		}

		log.Println("Successfully connected to Redis.")
		redisClient = client // Assign the successful client to the outer scope variable
		return nil           // Success
	}

	// Create an exponential backoff strategy
	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 2 * time.Minute // Stop retrying after 2 minutes

	// Retry the connection
	err := backoff.Retry(connectToRedis, bo)
	if err != nil {
		// If it still fails after all retries, then exit
		log.Fatalf("Could not connect to Redis after multiple retries: %v", err)
	}
	defer redisClient.Close()

	// Create session store
	sessionStore := session.NewRedisStore(redisClient, time.Duration(cfg.WebSocket.SessionTTL)*time.Second)

	// --- Dynamic Broker Initialization ---
	var messageBroker broker.MessageBroker

	log.Printf("Initializing message broker of type: %s", cfg.Broker.Type)
	switch strings.ToLower(cfg.Broker.Type) {
	case "redis":
		// The Redis broker can re-use the same client as the session store.
		messageBroker = broker.NewRedisBroker(redisClient)
	case "kafka":
		messageBroker, err = broker.NewKafkaBroker(cfg.Broker.Kafka.Brokers, cfg.Broker.Kafka.GroupID)
		if err != nil {
			log.Fatalf("Failed to create Kafka broker: %v", err)
		}
	default:
		// This should be caught by config validation, but we check again as a safeguard.
		log.Fatalf("Invalid broker type specified: %s", cfg.Broker.Type)
	}
	defer messageBroker.Close()
	// --- End of Broker Initialization ---

	// Auth Initialization
	var jwtValidator *websocket.JWTValidator
	if cfg.Auth.Enabled {
		jwtValidator = websocket.NewJWTValidator(&cfg.Auth, redisClient)
		log.Println("JWT Authentication is ENABLED.")
	} else {
		log.Println("JWT Authentication is DISABLED.")
	}
	// --- End of Auth Initialization ---

	// Create client manager
	clientManager := websocket.NewClientManager(sessionStore, serverID)

	// Initialize handlers
	handler := websocket.NewHandler(clientManager, messageBroker, jwtValidator, &cfg.Auth)

	// Create and configure server
	port := ":" + strconv.Itoa(cfg.Server.Port)
	srv := server.NewServer(port, handler.HandleWebSocket)

	// Start message listener
	go handler.ListenForResponses(ctx)

	// Start server
	go srv.Start()
	log.Println("WebSocket pooler started on " + port)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	log.Println("Shutdown signal received")

	// Graceful shutdown
	srv.Shutdown(ctx, clientManager, messageBroker)
}
