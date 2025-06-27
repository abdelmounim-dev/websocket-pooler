package config

import "github.com/spf13/viper"

func setDefaults() {
	// Server
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.readTimeout", 15)
	viper.SetDefault("server.writeTimeout", 15)

	// Auth
	viper.SetDefault("auth.enabled", false) // Default to off for security
	viper.SetDefault("auth.jwtSecret", "default-secret")
	viper.SetDefault("auth.tokenQueryParam", "token")
	viper.SetDefault("auth.revocationListKey", "jwt:revoked")

	// Redis
	viper.SetDefault("redis.address", "localhost:6379")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("redis.poolSize", 100)
	viper.SetDefault("redis.poolTimeout", 5)

	// Redis Channels
	viper.SetDefault("redis.channels.inbound", "ws:inbound")
	viper.SetDefault("redis.channels.outbound", "ws:outbound")
	viper.SetDefault("redis.channels.system", "ws:system")
	viper.SetDefault("redis.channels.connection", "ws:connections")

	// WebSocket
	viper.SetDefault("websocket.maxConnections", 10000)
	viper.SetDefault("websocket.messageSizeLimit", 2048)
	viper.SetDefault("websocket.reconnectBackoff", 1000)
	viper.SetDefault("websocket.maxRetries", 5)
	viper.SetDefault("websocket.handshakeTimeout", 10)
	viper.SetDefault("websocket.pingInterval", 25)
	viper.SetDefault("websocket.pongTimeout", 30)
	viper.SetDefault("websocket.activityTimeout", 60)
	viper.SetDefault("websocket.writeTimeout", 10)
	viper.SetDefault("websocket.keepAlive", true)
	viper.SetDefault("websocket.sessionTTL", 90)

	// Metrics
	viper.SetDefault("metrics.enabled", true)
	viper.SetDefault("metrics.port", 9090)
	viper.SetDefault("metrics.path", "/metrics")
}
