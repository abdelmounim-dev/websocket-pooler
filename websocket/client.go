package websocket

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/abdelmounim-dev/websocket-pooler/config"
	"github.com/cenkalti/backoff/v4"
	"github.com/gorilla/websocket"
)

const (
	websocketRetryDelay = 200 * time.Millisecond
)

// ClientSession represents a connected websocket client
type ClientSession struct {
	ID            string
	conn          *websocket.Conn
	ctx           context.Context
	cfg           *config.WebSocketConfig
	claims        *CustomClaims // Add this to store JWT claims
	lastActivity  atomic.Int64
	pingTicker    *time.Ticker
	activityTimer *time.Timer
	cancel        context.CancelFunc
	mu            sync.Mutex
	closeOnce     sync.Once
}

// NewClientSession creates a new client session
func NewClientSession(id string, conn *websocket.Conn, cfg *config.WebSocketConfig, claims *CustomClaims) *ClientSession {
	ctx, cancel := context.WithCancel(context.Background())
	cs := &ClientSession{
		ID:     id,
		conn:   conn,
		cfg:    cfg,
		claims: claims, // Set claims on creation
		cancel: cancel,
		ctx:    ctx,
	}
	cs.lastActivity.Store(time.Now().Unix())
	return cs
}

// CanAccess checks if the client's JWT scopes permit a certain action on a channel.
// Scopes are expected to be in the format "action:channel_pattern".
// e.g., "publish:user.*", "subscribe:updates"
func (s *ClientSession) CanAccess(action, channel string) bool {
	// If auth is disabled, claims will be nil. Default to allow.
	if s.claims == nil {
		return true
	}

	requiredScope := fmt.Sprintf("%s:%s", action, channel)

	for _, scope := range s.claims.Scopes {
		// Simple wildcard check (e.g., "publish:user.*" matches "publish:user.123")
		if strings.HasSuffix(scope, ".*") {
			prefix := strings.TrimSuffix(scope, ".*")
			// The required scope must start with the same action and channel prefix
			if strings.HasPrefix(requiredScope, prefix) {
				return true
			}
		}
		// Exact match
		if scope == requiredScope {
			return true
		}
	}
	return false
}

// SafeWriteJSON writes data to the websocket with retry capability
func (s *ClientSession) SafeWriteJSON(data interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	operation := func() error {
		return s.conn.WriteJSON(data)
	}

	backoffStrategy := backoff.WithContext(
		backoff.NewConstantBackOff(websocketRetryDelay),
		context.Background(),
	)

	return backoff.RetryNotify(operation, backoffStrategy, func(err error, d time.Duration) {
		log.Printf("Retrying WebSocket write: %v (next attempt in %s)", err, d)
	})
}

// UpdateActivity updates the last activity timestamp and resets the timeout timer
// This should only be called for actual client messages, not pong responses
func (s *ClientSession) UpdateActivity() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastActivity.Store(time.Now().Unix())

	// Reset the activity timer
	if s.activityTimer != nil {
		s.activityTimer.Stop()
		s.activityTimer = time.AfterFunc(
			time.Duration(s.cfg.ActivityTimeout)*time.Second,
			s.onActivityTimeout,
		)
	}
}

// LastActivityTime returns the time of last activity
func (s *ClientSession) LastActivityTime() time.Time {
	return time.Unix(s.lastActivity.Load(), 0)
}

func (s *ClientSession) StartTimers() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.activityTimer = time.AfterFunc(
		time.Duration(s.cfg.ActivityTimeout)*time.Second,
		s.onActivityTimeout,
	)

	// Remove ping mechanism if you want pure inactivity timeout
	s.pingTicker = time.NewTicker(
		time.Duration(s.cfg.PingInterval) * time.Second,
	)
	go s.pingLoop()
}

func (s *ClientSession) pingLoop() {
	defer s.pingTicker.Stop()

	for {
		select {
		case <-s.pingTicker.C:
			if err := s.SendPing(); err != nil {
				log.Printf("Failed to send ping to %s: %v", s.ID, err)
				s.Close(websocket.CloseInternalServerErr, "Ping failure")
				return
			}
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *ClientSession) onActivityTimeout() {
	log.Printf("Connection %s timed out", s.ID)
	s.Close(websocket.ClosePolicyViolation, "Inactivity timeout")
}

func (s *ClientSession) SendPing() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.conn.WriteControl(
		websocket.PingMessage,
		[]byte{},
		time.Now().Add(time.Duration(s.cfg.WriteTimeout)*time.Second),
	)
}

// UpdateLastSeen updates only the timestamp (for pong responses)
// Does NOT reset the activity timer
func (s *ClientSession) UpdateLastSeen() {
	s.lastActivity.Store(time.Now().Unix())
}

// GetPongHandler returns a pong handler function based on configuration
func (s *ClientSession) GetPongHandler() func(string) error {
	return func(msg string) error {
		if s.cfg.KeepAlive {
			s.UpdateActivity() // Reset timeout timer
		} else {
			s.UpdateLastSeen() // Just update timestamp
		}

		// Optional: log pong messages (consider removing in production)
		if len(msg) > 0 {
			log.Printf("Pong received from client %s: %s", s.ID, msg)
		}

		return nil
	}
}

// Close closes the websocket connection idempotently.
func (s *ClientSession) Close(code int, text string) error {
	s.closeOnce.Do(func() {
		log.Printf("Closing connection for client %s: code=%d, reason='%s'", s.ID, code, text)

		// This entire block of code will now run exactly once.
		s.mu.Lock()
		defer s.mu.Unlock()

		// Stop timers
		if s.pingTicker != nil {
			s.pingTicker.Stop()
		}
		if s.activityTimer != nil {
			s.activityTimer.Stop()
		}

		// Cancel context to signal all goroutines to stop
		if s.cancel != nil {
			s.cancel()
		}

		writeTimeout := time.Duration(s.cfg.WriteTimeout) * time.Second
		// Attempt to send the close message but do not log the specific "close sent" error,
		// as it's an expected outcome of a race condition where the client closes first.
		_ = s.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(code, text),
			time.Now().Add(writeTimeout),
		)

		// Finally, close the underlying connection.
		// Add a small delay to allow the close frame to be sent and received.
		time.Sleep(100 * time.Millisecond)
		s.conn.Close()
	})
	return nil
}
