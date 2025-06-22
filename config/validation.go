package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

func (c *AppConfig) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return errors.New("invalid server port")
	}
	// Validate auth config
	if c.Auth.Enabled {
		if c.Auth.JWTSecret == "" || c.Auth.JWTSecret == "default-secret" {
			return errors.New("auth.jwtSecret must be set to a strong secret when auth is enabled")
		}
		if c.Auth.TokenQueryParam == "" {
			return errors.New("auth.tokenQueryParam must be configured when auth is enabled")
		}
	}

	// Validate broker configuration
	switch strings.ToLower(c.Broker.Type) {
	case "redis":
		if c.Broker.Redis.Address == "" {
			return errors.New("redis address must be specified for redis broker")
		}
		if c.Broker.Redis.Channels.Inbound == "" || c.Broker.Redis.Channels.Outbound == "" {
			return errors.New("redis channels must be configured for redis broker")
		}
	case "kafka":
		if len(c.Broker.Kafka.Brokers) == 0 {
			return errors.New("kafka brokers must be specified for kafka broker")
		}
		if c.Broker.Kafka.GroupID == "" {
			return errors.New("kafka groupID must be specified for kafka broker")
		}
	default:
		return fmt.Errorf("invalid broker type: %s. Must be 'redis' or 'kafka'", c.Broker.Type)
	}

	if c.WebSocket.MaxConnections < 1 {
		return errors.New("max connections must be positive")
	}

	if c.WebSocket.HandshakeTimeout < 1 {
		return errors.New("handshake timeout must be at least 1 second")
	}

	if c.WebSocket.PingInterval >= c.WebSocket.ActivityTimeout {
		return errors.New("ping interval should be less than activity timeout")
	}

	if c.WebSocket.SessionTTL <= c.WebSocket.ActivityTimeout {
		return errors.New("session TTL should be greater than activity timeout")
	}

	return nil
}

func bindEnvVars() {
	// Server
	viper.BindEnv("server.port", "WSGATEWAY_PORT")

	// Auth
	viper.BindEnv("auth.enabled", "WSGATEWAY_AUTH_ENABLED")
	viper.BindEnv("auth.jwtSecret", "WSGATEWAY_AUTH_JWT_SECRET")
	viper.BindEnv("auth.tokenQueryParam", "WSGATEWAY_AUTH_TOKEN_PARAM")
	viper.BindEnv("auth.revocationListKey", "WSGATEWAY_AUTH_REVOCATION_KEY")

	// Broker
	viper.BindEnv("broker.type", "WSGATEWAY_BROKER_TYPE")
	viper.BindEnv("broker.redis.address", "WSGATEWAY_REDIS_ADDRESS")
	viper.BindEnv("broker.redis.password", "WSGATEWAY_REDIS_PASSWORD")
	viper.BindEnv("broker.redis.channels.inbound", "WSGATEWAY_REDIS_INBOUND_CHANNEL")
	viper.BindEnv("broker.redis.channels.outbound", "WSGATEWAY_REDIS_OUTBOUND_CHANNEL")
	viper.BindEnv("broker.kafka.brokers", "WSGATEWAY_KAFKA_BROKERS")
	viper.BindEnv("broker.kafka.groupID", "WSGATEWAY_KAFKA_GROUPID")

	// WebSocket
	viper.BindEnv("websocket.maxConnections", "WSGATEWAY_MAX_CONNECTIONS")
	viper.BindEnv("websocket.handshakeTimeout", "WSGATEWAY_HANDSHAKE_TIMEOUT")
	viper.BindEnv("websocket.pingInterval", "WSGATEWAY_PING_INTERVAL")
	viper.BindEnv("websocket.pongTimeout", "WSGATEWAY_PONG_TIMEOUT")
	viper.BindEnv("websocket.activityTimeout", "WSGATEWAY_ACTIVITY_TIMEOUT")
	viper.BindEnv("websocket.writeTimeout", "WSGATEWAY_WRITE_TIMEOUT")
	viper.BindEnv("websocket.sessionTTL", "WSGATEWAY_SESSION_TTL")
}
