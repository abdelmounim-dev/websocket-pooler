package websocket

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/abdelmounim-dev/websocket-pooler/broker"
	"github.com/abdelmounim-dev/websocket-pooler/config"
)

// Constants for channel names
const (
	BackendRequestsChannel  = "backend-requests"
	BackendResponsesChannel = "backend-responses"
)

// Upgrader for websocket connections
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Handler manages websocket connections and message routing
type Handler struct {
	manager *ClientManager
	broker  broker.MessageBroker
}

// NewHandler creates a new websocket handler
func NewHandler(manager *ClientManager, broker broker.MessageBroker) *Handler {
	return &Handler{
		manager: manager,
		broker:  broker,
	}
}

// HandleWebSocket handles incoming websocket connections
func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	// Generate a unique client ID
	clientID := uuid.New().String()

	// Create a new client session
	session := NewClientSession(clientID, conn, &config.Get().WebSocket)
	session.StartTimers()

	// Add client to manager (which handles in-memory and persistent storage)
	if err := h.manager.AddClient(r.Context(), session); err != nil {
		conn.Close()
		return
	}

	// Defer cleanup for when this function returns (i.e., connection closes)
	defer h.manager.RemoveClient(clientID)

	// Configure pong handler
	conn.SetPongHandler(session.GetPongHandler())

	// Send client ID to client
	if err := session.SafeWriteJSON(map[string]string{"client_id": clientID}); err != nil {
		log.Printf("Failed to send client ID: %v", err)
		conn.Close()
		// defer will handle the cleanup
		return
	}

	// Read messages from client
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) &&
				!errors.Is(err, net.ErrClosed) {
				log.Printf("Read error from client %s: %v", clientID, err)
			}
			break
		}

		// Update activity timestamp on the client session
		session.UpdateActivity()

		// Refresh session TTL in Redis
		h.manager.RefreshSessionTTL(r.Context(), clientID)

		// Forward message to backend
		h.manager.IncreaseWaitGroup()
		go func() {
			defer h.manager.DecreaseWaitGroup()

			ctxTimeout, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()

			// The message broker needs the serverID
			if err := h.broker.Publish(ctxTimeout, BackendRequestsChannel, broker.Message{
				ClientID: clientID,
				ServerID: h.manager.serverID, // Get serverID from manager
				Data:     string(msg),
			}); err != nil {
				log.Printf("Failed to publish message for client %s: %v", clientID, err)
			}
		}()
	}
	// No explicit cleanup here, defer handles it.
}

// ListenForResponses listens for messages from backend and routes them to clients
func (h *Handler) ListenForResponses(ctx context.Context) {
	messageChan, err := h.broker.Subscribe(ctx, BackendResponsesChannel)
	if err != nil {
		log.Fatalf("Failed to subscribe to %s: %v", BackendResponsesChannel, err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-messageChan:
			if !ok {
				log.Println("Backend response channel closed")
				return
			}

			// Only process messages destined for this server instance
			if message.ServerID != h.manager.serverID {
				continue
			}

			clientID := message.ClientID

			if session, ok := h.manager.GetClient(clientID); ok {
				if err := session.SafeWriteJSON(message.Data); err != nil {
					log.Printf("Failed to send message to client %s: %v", clientID, err)
					session.Close(websocket.CloseInternalServerErr, "Failed to send message")
					h.manager.RemoveClient(clientID)
				}
			}
		}
	}
}
