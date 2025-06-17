package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/abdelmounim-dev/websocket-pooler/broker"
	"github.com/abdelmounim-dev/websocket-pooler/config"
	"github.com/abdelmounim-dev/websocket-pooler/server"
	"github.com/abdelmounim-dev/websocket-pooler/services"
	"github.com/abdelmounim-dev/websocket-pooler/session"
	"github.com/abdelmounim-dev/websocket-pooler/websocket"
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
	// Generate a unique ID for this server instance
	serverID := uuid.New().String()
	log.Printf("Starting server instance with ID: %s", serverID)

	// TODO: separate Redis client creation and session store initialization and broker in some way make client singleton
	// Create Redis Client
	redisClient, err := services.NewRedisClient(cfg.Redis.Address, cfg.Redis.Password, cfg.Redis.DB, cfg.Redis.PoolSize, cfg.Redis.PoolTimeout)
	if err != nil {
		log.Fatalf("Failed to create Redis client: %v", err)
	}

	// Create session store
	sessionStore := session.NewRedisStore(redisClient, time.Duration(cfg.WebSocket.SessionTTL)*time.Second)

	// Create message broker
	messageBroker, err := broker.NewRedisBroker(cfg.Redis.Address, cfg.Redis.Password)
	if err != nil {
		log.Fatalf("Failed to create Redis broker: %v", err)
	}
	defer messageBroker.Close()

	// Create client manager
	clientManager := websocket.NewClientManager(sessionStore, serverID)

	// Initialize handlers
	handler := websocket.NewHandler(clientManager, messageBroker)

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
