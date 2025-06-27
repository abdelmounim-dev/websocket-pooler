package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/abdelmounim-dev/websocket-pooler/broker"
	"github.com/abdelmounim-dev/websocket-pooler/config"
	"github.com/abdelmounim-dev/websocket-pooler/metrics"
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
	manager      *ClientManager
	broker       broker.MessageBroker
	jwtValidator *JWTValidator      // Add validator
	authConfig   *config.AuthConfig // Add auth config
}

// NewHandler creates a new websocket handler
func NewHandler(manager *ClientManager, broker broker.MessageBroker, jwtValidator *JWTValidator, authConfig *config.AuthConfig) *Handler {
	return &Handler{
		manager:      manager,
		broker:       broker,
		jwtValidator: jwtValidator,
		authConfig:   authConfig,
	}
}

// HandleWebSocket handles incoming websocket connections
func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	var claims *CustomClaims
	var err error

	// --- Handshake Authentication ---
	if h.authConfig.Enabled {
		if h.jwtValidator == nil {
			log.Printf("Auth Error: Auth is enabled but JWT validator is not initialized.")
			http.Error(w, "Internal server configuration error", http.StatusInternalServerError)
			return
		}

		tokenString := r.URL.Query().Get(h.authConfig.TokenQueryParam)
		if tokenString == "" {
			log.Printf("Auth Error: Missing token in request from %s", r.RemoteAddr)
			http.Error(w, "Missing authentication token", http.StatusUnauthorized)
			return
		}

		claims, err = h.jwtValidator.ValidateToken(r.Context(), tokenString)
		if err != nil {
			log.Printf("Auth Error: Invalid token from %s. Reason: %v", r.RemoteAddr, err)
			http.Error(w, "Invalid authentication token", http.StatusUnauthorized)
			return
		}
		log.Printf("Client authenticated successfully. Subject: %s", claims.Subject)
	}
	// --- End Handshake Authentication ---

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	// Use subject from JWT as clientID if available, otherwise generate a new one.
	var clientID string
	if claims != nil && claims.Subject != "" {
		clientID = claims.Subject
	} else {
		clientID = uuid.New().String()
	}

	// Create a new client session, passing claims
	session := NewClientSession(clientID, conn, &config.Get().WebSocket, claims)
	session.StartTimers()

	// Add client to manager
	if err := h.manager.AddClient(r.Context(), session); err != nil {
		conn.Close()
		return
	}
	defer h.manager.RemoveClient(clientID)
	conn.SetPongHandler(session.GetPongHandler())

	// Send client ID to client for reference
	if err := session.SafeWriteJSON(map[string]string{"client_id": clientID}); err != nil {
		log.Printf("Failed to send client ID: %v", err)
		return // defer will handle cleanup
	}

	// Read messages from client
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) &&
				!errors.Is(err, net.ErrClosed) {
				log.Printf("Read error from client %s: %v", clientID, err)
			}
			session.Close(websocket.CloseNormalClosure, "Client disconnected")
			break
		}
		metrics.MessagesReceived.Inc()
		session.UpdateActivity()

		// --- Channel Access Control ---
		if h.authConfig.Enabled {
			var req struct {
				Action  string `json:"action"`
				Channel string `json:"channel"`
			}
			// Best-effort unmarshal to check for action/channel based auth.
			if json.Unmarshal(msg, &req) == nil && req.Action != "" && req.Channel != "" {
				if !session.CanAccess(req.Action, req.Channel) {
					log.Printf("Authorization DENIED for client %s: action '%s' on channel '%s'", clientID, req.Action, req.Channel)
					session.SafeWriteJSON(map[string]string{
						"error":   "forbidden",
						"details": fmt.Sprintf("action '%s' on channel '%s' not allowed", req.Action, req.Channel),
					})
					continue // Do not process or forward the unauthorized message
				}
			}
		}
		// --- End Channel Access Control ---

		h.manager.RefreshSessionTTL(r.Context(), clientID)

		// Forward message to backend
		h.manager.IncreaseWaitGroup()
		go func() {
			defer h.manager.DecreaseWaitGroup()
			ctxTimeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := h.broker.Publish(ctxTimeout, BackendRequestsChannel, broker.Message{

				ClientID: clientID,
				ServerID: h.manager.serverID,
				Data:     string(msg),
			}); err != nil {
				log.Printf("Failed to publish message for client %s: %v", clientID, err)
			}
		}()
		metrics.BrokerMessagesPublished.WithLabelValues(h.broker.Type()).Inc()
	}
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
				metrics.MessagesSent.Inc()
				if err := session.SafeWriteJSON(message.Data); err != nil {
					log.Printf("Failed to send message to client %s: %v", clientID, err)
					session.Close(websocket.CloseInternalServerErr, "Failed to send message")
					h.manager.RemoveClient(clientID)
				}
			}
		}
	}
}
