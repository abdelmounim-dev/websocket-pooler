package websocket

import (
	"context"
	"sync"
	"time"

	"github.com/abdelmounim-dev/websocket-pooler/metrics"
	"github.com/abdelmounim-dev/websocket-pooler/session"
	"github.com/gorilla/websocket"
)

// ClientManager manages connected websocket clients for a single server instance.
// It coordinates between the in-memory connection map and the persistent session store.
type ClientManager struct {
	clients      sync.Map // In-memory map of active connections for this instance
	wg           sync.WaitGroup
	sessionStore session.Store
	serverID     string
}

// NewClientManager creates a new client manager.
func NewClientManager(store session.Store, serverID string) *ClientManager {
	return &ClientManager{
		clients:      sync.Map{},
		sessionStore: store,
		serverID:     serverID,
	}
}

// AddClient adds a client to the manager, storing the connection in-memory
// and creating a session record in the persistent store.
func (m *ClientManager) AddClient(ctx context.Context, clientSession *ClientSession) error {
	// Create the persistent session record first
	sessionInfo := &session.Session{
		ClientID:    clientSession.ID,
		ServerID:    m.serverID,
		ConnectedAt: time.Now(),
	}
	if err := m.sessionStore.Create(ctx, sessionInfo); err != nil {
		log.Printf("Failed to create session in store for client %s: %v", clientSession.ID, err)
		return err
	}

	// If successful, store the live connection in the local map
	m.clients.Store(clientSession.ID, clientSession)
	metrics.ActiveConnections.Inc()
	metrics.TotalConnections.Inc()
	log.Printf("Client %s connected to server %s and session created in Redis", clientSession.ID, m.serverID)
	return nil
}

// RemoveClient removes a client from the in-memory map and the persistent store.
func (m *ClientManager) RemoveClient(clientID string) {
	// Remove from the local map first
	m.clients.Delete(clientID)

	// Then, remove from the persistent store.
	// Use a background context as the original request context may be cancelled.
	if err := m.sessionStore.Delete(context.Background(), clientID); err != nil {
		log.Printf("Failed to delete session from store for client %s: %v", clientID, err)
	}
	metrics.ActiveConnections.Dec()
	log.Printf("Client %s disconnected and session deleted from Redis", clientID)
}

// GetClient retrieves a live client connection by ID from the in-memory map.
func (m *ClientManager) GetClient(clientID string) (*ClientSession, bool) {
	if client, ok := m.clients.Load(clientID); ok {
		return client.(*ClientSession), true
	}
	return nil, false
}

// RefreshSessionTTL updates the TTL of the client's session in the persistent store.
func (m *ClientManager) RefreshSessionTTL(ctx context.Context, clientID string) {
	if err := m.sessionStore.RefreshTTL(ctx, clientID); err != nil {
		// Log the error but don't disconnect the client for this.
		// It might be a transient Redis issue.
		log.Printf("Failed to refresh session TTL for client %s: %v", clientID, err)
	}
}

// IncreaseWaitGroup increases the wait group counter
func (m *ClientManager) IncreaseWaitGroup() {
	m.wg.Add(1)
}

// DecreaseWaitGroup decreases the wait group counter
func (m *ClientManager) DecreaseWaitGroup() {
	m.wg.Done()
}

// WaitForCompletion waits for all operations to complete
func (m *ClientManager) WaitForCompletion() {
	m.wg.Wait()
}

// CloseAllConnections sends close messages to all clients and removes them
func (m *ClientManager) CloseAllConnections(reason string) {
	m.clients.Range(func(key, value interface{}) bool {
		clientID := key.(string)
		session := value.(*ClientSession)

		log.Printf("Closing connection for client %s: %s", clientID, reason)
		session.Close(websocket.CloseGoingAway, reason)
		// RemoveClient will also handle deleting from the session store
		m.RemoveClient(clientID)

		return true
	})
}
