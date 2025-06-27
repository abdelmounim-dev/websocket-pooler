package main

import (
	"context"
	"encoding/json"
	"log"
	"os" // Imported the 'os' package
	"time"

	"github.com/go-redis/redis/v8"
)

// Message struct MUST match the broker's message structure.
type Message struct {
	ClientID string      `json:"client_id"`
	ServerID string      `json:"server_id"`
	Data     interface{} `json:"data"`
}

// Session struct represents the data stored in Redis for each client session.
// It must match the structure saved by the pooler's session.RedisStore.
// This is no longer needed for the echo logic but kept for context.
type Session struct {
	ClientID    string    `json:"client_id"`
	ServerID    string    `json:"server_id"`
	ConnectedAt time.Time `json:"connected_at"`
}

// MarshalBinary implements the encoding.BinaryMarshaler interface for Redis.
func (m Message) MarshalBinary() ([]byte, error) {
	return json.Marshal(m)
}

// UnmarshalBinary implements the encoding.BinaryUnmarshaler interface for Redis.
func (m *Message) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, m)
}

// getEnv gets an environment variable or returns a default value.
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func main() {
	// --- MODIFIED SECTION ---
	// Get Redis address from environment variable, with a fallback.
	redisAddr := getEnv("REDIS_ADDRESS", "localhost:6379")
	log.Printf("Connecting to Redis at %s", redisAddr)

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	// --- END MODIFIED SECTION ---

	ctx := context.Background()

	pubsub := rdb.Subscribe(ctx, "backend-requests")
	defer pubsub.Close()

	log.Println("Test backend started. Listening for requests...")

	for msg := range pubsub.Channel() {
		var request Message
		if err := json.Unmarshal([]byte(msg.Payload), &request); err != nil {
			log.Printf("Error decoding request: %v", err)
			continue
		}

		log.Printf("Received message from client %s (via server %s): %v", request.ClientID, request.ServerID, request.Data)

		// --- ECHO LOGIC ---
		// We no longer scan for all clients. We use the sender's info directly.
		log.Printf("Echoing message back to client %s", request.ClientID)

		// 1. Create the response message targeting the original sender.
		//    The ClientID and ServerID from the request are used to route
		//    the response back to the correct client via the correct server.
		responseMsg := Message{
			ClientID: request.ClientID,
			ServerID: request.ServerID,
			Data:     request.Data, // Echo back the original data
		}

		// 2. Publish the single response message to the `backend-responses` channel.
		err := rdb.Publish(ctx, "backend-responses", responseMsg).Err()
		if err != nil {
			log.Printf("Error echoing to client %s: %v", request.ClientID, err)
		}
	}
}
