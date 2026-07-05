// internal/config/config.go
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Env           string
	GRPC          GRPCConfig
	HTTP          HTTPConfig
	Postgres      PostgresConfig
	Shutdown      time.Duration
	JWT           JWTConfig
	Kafka         KafkaConfig
	Observability ObservabilityConfig
	Redis         RedisConfig
}

type GRPCConfig struct{ Port string }
type HTTPConfig struct{ Port string }

type PostgresConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DB       string
	SSLMode  string
}

type JWTConfig struct {
	Secret string
	TTL    time.Duration
}

type KafkaConfig struct {
	Brokers         []string
	Topic           string
	ConsumerGroupID string
	RelayInterval   time.Duration
}

type ObservabilityConfig struct {
	OTLPEndpoint   string
	ServiceName    string
	TracingEnabled bool
}

type RedisConfig struct {
	Addr string
}

// DSN собирает строку подключения к PostgreSQL в URL-формате.
func (p PostgresConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		p.User, p.Password, p.Host, p.Port, p.DB, p.SSLMode,
	)
}

// Load читает конфигурацию из переменных окружения.
func Load() (*Config, error) {
	var missing []string
	req := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}

	cfg := &Config{
		Env:  getEnv("APP_ENV", "local"),
		GRPC: GRPCConfig{Port: getEnv("GRPC_PORT", "50051")},
		HTTP: HTTPConfig{Port: getEnv("HTTP_PORT", "8080")},
		Postgres: PostgresConfig{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getEnv("POSTGRES_PORT", "5432"),
			User:     req("POSTGRES_USER"),
			Password: req("POSTGRES_PASSWORD"),
			DB:       req("POSTGRES_DB"),
			SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
		},
		Shutdown: getEnvDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		JWT: JWTConfig{
			Secret: req("JWT_SECRET"),
			TTL:    getEnvDuration("JWT_TTL", 15*time.Minute),
		},
		Kafka: KafkaConfig{
			Brokers:         strings.Split(getEnv("KAFKA_BROKERS", "localhost:9092"), ","),
			Topic:           getEnv("KAFKA_TOPIC", "wallet.transactions.completed"),
			ConsumerGroupID: getEnv("KAFKA_CONSUMER_GROUP", "notifier"),
			RelayInterval:   getEnvDuration("KAFKA_RELAY_INTERVAL", 500*time.Millisecond),
		},
		Observability: ObservabilityConfig{
			OTLPEndpoint:   getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
			ServiceName:    getEnv("OTEL_SERVICE_NAME", "t-wallet"),
			TracingEnabled: getEnv("TRACING_ENABLED", "true") == "true",
		},
		Redis: RedisConfig{
			Addr: getEnv("REDIS_ADDR", "localhost:6379"),
		},
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("не заданы обязательные env-переменные: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}