package config

import (
	"errors"

	"github.com/spf13/viper"
)

func (c *AppConfig) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return errors.New("invalid server port")
	}

	if c.Redis.Address == "" {
		return errors.New("redis address must be specified")
	}

	if c.WebSocket.MaxConnections < 1 {
		return errors.New("max connections must be positive")
	}

	if c.Redis.Channels.Inbound == "" || c.Redis.Channels.Outbound == "" {
		return errors.New("redis channels must be configured")
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

	// Redis
	viper.BindEnv("redis.address", "WSGATEWAY_REDIS_ADDRESS")
	viper.BindEnv("redis.password", "WSGATEWAY_REDIS_PASSWORD")
	viper.BindEnv("redis.channels.inbound", "WSGATEWAY_REDIS_INBOUND_CHANNEL")
	viper.BindEnv("redis.channels.outbound", "WSGATEWAY_REDIS_OUTBOUND_CHANNEL")

	// WebSocket
	viper.BindEnv("websocket.maxConnections", "WSGATEWAY_MAX_CONNECTIONS")
	viper.BindEnv("websocket.handshakeTimeout", "WSGATEWAY_HANDSHAKE_TIMEOUT")
	viper.BindEnv("websocket.pingInterval", "WSGATEWAY_PING_INTERVAL")
	viper.BindEnv("websocket.pongTimeout", "WSGATEWAY_PONG_TIMEOUT")
	viper.BindEnv("websocket.activityTimeout", "WSGATEWAY_ACTIVITY_TIMEOUT")
	viper.BindEnv("websocket.writeTimeout", "WSGATEWAY_WRITE_TIMEOUT")
	viper.BindEnv("websocket.sessionTTL", "WSGATEWAY_SESSION_TTL")
}
