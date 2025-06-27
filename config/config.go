package config

import (
	"fmt"
	"sync"

	"github.com/spf13/viper"
)

type AppConfig struct {
	Server    ServerConfig
	Broker    BrokerConfig // Changed from Redis to Broker
	WebSocket WebSocketConfig
	Auth      AuthConfig
	Metrics   MetricsConfig
}

type MetricsConfig struct {
	Enabled bool
	Port    int
	Path    string
}

type AuthConfig struct {
	Enabled           bool   `mapstructure:"enabled"`
	JWTSecret         string `mapstructure:"jwtSecret"`
	TokenQueryParam   string `mapstructure:"tokenQueryParam"`
	RevocationListKey string `mapstructure:"revocationListKey"`
}

type ServerConfig struct {
	Port         int
	ReadTimeout  int
	WriteTimeout int
}

// BrokerConfig holds settings for the message broker choice
type BrokerConfig struct {
	Type  string
	Redis RedisConfig
	Kafka KafkaConfig
}

type RedisConfig struct {
	Address     string
	Password    string
	DB          int
	Channels    RedisChannels
	PoolSize    int
	PoolTimeout int
}

type KafkaConfig struct {
	Brokers []string
	GroupID string
}

type RedisChannels struct {
	Inbound    string
	Outbound   string
	System     string
	Connection string
}

type WebSocketConfig struct {
	MaxConnections   int
	MessageSizeLimit int
	HandshakeTimeout int
	PingInterval     int // Seconds
	PongTimeout      int // Seconds
	ActivityTimeout  int // Seconds
	WriteTimeout     int // Seconds
	ReconnectBackoff int // Milliseconds
	MaxRetries       int
	KeepAlive        bool
	SessionTTL       int // Seconds
}

type Timeouts struct {
	HandshakeTimeout int
	PingInterval     int
	PongTimeout      int
	ActivityTimeout  int
}

var (
	instance *AppConfig
	once     sync.Once
)

func Initialize(env string) error {
	var initErr error
	once.Do(func() {
		viper.SetConfigName(fmt.Sprintf("config.%s", env))
		viper.SetConfigType("yaml")
		viper.AddConfigPath("./configs")
		viper.AddConfigPath(".")
		viper.AddConfigPath("/app")

		viper.AutomaticEnv()
		viper.SetEnvPrefix("WSGATEWAY")

		setDefaults()
		bindEnvVars()

		if err := viper.ReadInConfig(); err != nil {
			initErr = fmt.Errorf("config file error: %w", err)
			return
		}

		if err := viper.Unmarshal(&instance); err != nil {
			initErr = fmt.Errorf("config unmarshal error: %w", err)
			return
		}

		if err := instance.Validate(); err != nil {
			initErr = fmt.Errorf("config validation failed: %w", err)
			return
		}
	})
	return initErr
}

func Get() *AppConfig {
	return instance
}
