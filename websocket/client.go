package websocket

import (
	"context"
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
	lastActivity  atomic.Int64
	pingTicker    *time.Ticker
	activityTimer *time.Timer
	cancel        context.CancelFunc
	mu            sync.Mutex
}

// NewClientSession creates a new client session
func NewClientSession(id string, conn *websocket.Conn, cfg *config.WebSocketConfig) *ClientSession {
	ctx, cancel := context.WithCancel(context.Background())
	cs := &ClientSession{
		ID:     id,
		conn:   conn,
		cfg:    cfg,
		cancel: cancel,
		ctx:    ctx,
	}
	cs.lastActivity.Store(time.Now().Unix())
	return cs
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

// Close closes the websocket connection
func (s *ClientSession) Close(code int, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop timers
	if s.pingTicker != nil {
		s.pingTicker.Stop()
	}
	if s.activityTimer != nil {
		s.activityTimer.Stop()
	}

	// Cancel context
	if s.cancel != nil {
		s.cancel()
	}

	writeTimeout := time.Duration(s.cfg.WriteTimeout) * time.Second
	err := s.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(code, text),
		time.Now().Add(writeTimeout),
	)
	if err != nil {
		log.Printf("Error sending close message: %v", err)
	}

	return s.conn.Close()
}
