// /home/ab/ws-pooler/tests/integration/integration_test.go
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/abdelmounim-dev/websocket-pooler/broker"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	wsURL                  = "ws://localhost:8080"
	redisAddr              = "localhost:6379"
	backendRequestsChannel = "backend-requests"  // Channel where pooler sends messages
	backendResponsesChannel= "backend-responses" // Channel where backend sends messages
	testTimeout            = 15 * time.Second
)

// mockBackend simulates a backend service by listening on the inbound Redis channel
// and sending responses to the outbound channel.
func mockBackend(ctx context.Context, t *testing.T, redisClient *redis.Client) {
	pubsub := redisClient.Subscribe(ctx, backendRequestsChannel)
	defer pubsub.Close()

	log.Println("Mock backend started, listening for messages...")

	for {
		select {
		case <-ctx.Done():
			log.Println("Mock backend shutting down.")
			return
		case msg, ok := <-pubsub.Channel():
			if !ok {
				return
			}
			
			var receivedMsg broker.Message
			err := json.Unmarshal([]byte(msg.Payload), &receivedMsg)
			require.NoError(t, err, "Backend failed to unmarshal message from pooler")

			log.Printf("Mock backend received message for client %s", receivedMsg.ClientID)

			// Echo the data back to the client via the responses channel
			response := broker.Message{
				ClientID: receivedMsg.ClientID,
				ServerID: receivedMsg.ServerID, // Must be the same server to route back
				Data:     receivedMsg.Data,     // Echo the original data
			}
			
			payload, err := json.Marshal(response)
			require.NoError(t, err, "Backend failed to marshal response message")

			err = redisClient.Publish(ctx, backendResponsesChannel, payload).Err()
			require.NoError(t, err, "Backend failed to publish response to Redis")

			log.Printf("Mock backend sent echo response for client %s", response.ClientID)
		}
	}
}

func TestE2EMessageFlow(t *testing.T) {
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("Skipping integration test: set INTEGRATION env var to run")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Setup Redis client for mock backend
	redisClient := redis.NewClient(&redis.Options{Addr: redisAddr})
	require.NoError(t, redisClient.Ping(ctx).Err(), "Failed to connect to Redis")
	defer redisClient.Close()
	
	// Start mock backend in a goroutine
	go mockBackend(ctx, t, redisClient)
	
	// Setup WebSocket client
	u := url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/"}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err, "Failed to connect to WebSocket server")
	defer conn.Close()

	log.Println("WebSocket client connected.")

	// 1. Contract Test: Verify initial message from server
	var initialMsg map[string]string
	err = conn.ReadJSON(&initialMsg)
	require.NoError(t, err)
	assert.NotEmpty(t, initialMsg["client_id"], "Server should send a client_id on connect")
	log.Printf("Received client_id: %s", initialMsg["client_id"])

	// 2. Send a message to the backend
	testMessage := fmt.Sprintf("hello from integration test at %s", time.Now())
	messageToSend := map[string]string{"data": testMessage}
	err = conn.WriteJSON(messageToSend)
	require.NoError(t, err, "Failed to write message to WebSocket")
	log.Printf("Client sent message: %s", testMessage)

	// 3. Wait for the echo response from the mock backend
	var receivedEcho map[string]interface{}
	err = conn.ReadJSON(&receivedEcho)
	require.NoError(t, err, "Failed to read echo message from WebSocket")
	
	log.Printf("Client received echo: %v", receivedEcho)
	
	// 4. Assert the response is correct
	// The pooler forwards the `Data` field, which was a JSON string itself
	var innerData map[string]string
	err = json.Unmarshal([]byte(receivedEcho["data"].(string)), &innerData)
	require.NoError(t, err)

	assert.Equal(t, testMessage, innerData["data"], "Echoed data does not match original message")
}
