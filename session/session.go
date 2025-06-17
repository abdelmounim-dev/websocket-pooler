package session

import (
	"context"
	"time"
)

// Session holds metadata about a client's connection.
// This is the data that will be stored in a persistent store like Redis.
type Session struct {
	ClientID    string    `json:"client_id"`
	ServerID    string    `json:"server_id"` // ID of the pooler instance handling the connection
	ConnectedAt time.Time `json:"connected_at"`
}

// Store defines the interface for session management.
type Store interface {
	// Create stores a new session.
	Create(ctx context.Context, session *Session) error
	// Get retrieves a session by client ID.
	Get(ctx context.Context, clientID string) (*Session, error)
	// Delete removes a session.
	Delete(ctx context.Context, clientID string) error
	// RefreshTTL extends the session's lifetime in the store.
	RefreshTTL(ctx context.Context, clientID string) error
}
