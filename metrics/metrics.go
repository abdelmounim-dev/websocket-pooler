// File: metrics/metrics.go
package metrics

import (
	"fmt"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// WebSocket Metrics
	ActiveConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ws_connections_active",
		Help: "The current number of active WebSocket connections.",
	})
	TotalConnections = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ws_connections_total",
		Help: "The total number of WebSocket connections accepted.",
	})
	MessagesReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ws_messages_received_total",
		Help: "The total number of messages received from clients.",
	})
	MessagesSent = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ws_messages_sent_total",
		Help: "The total number of messages sent to clients.",
	})

	// Broker Metrics
	BrokerMessagesPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "broker_messages_published_total",
		Help: "The total number of messages published to the message broker.",
	}, []string{"broker_type"})
	BrokerPublishRetries = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "broker_publish_retries_total",
		Help: "The total number of retries when publishing to the message broker.",
	}, []string{"broker_type"})

	// Auth Metrics
	AuthSuccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "auth_success_total",
		Help: "The total number of successful authentications.",
	})
	AuthFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "auth_failures_total",
		Help: "The total number of failed authentications.",
	}, []string{"reason"})
)

// StartServer starts the HTTP server for Prometheus metrics.
func StartServer(port int, path string) {
	http.Handle(path, promhttp.Handler())

	addr := fmt.Sprintf(":%d", port)
	log.Printf("Starting metrics server on %s%s", addr, path)

	go func() {
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Fatalf("Failed to start metrics server: %v", err)
		}
	}()
}
