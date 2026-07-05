// cmd/notifier/main.go
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Sushiiis/T-Wallet/internal/config"
	"github.com/Sushiiis/T-Wallet/internal/kafka/consumer"
	"github.com/Sushiiis/T-Wallet/internal/repository/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("notifier остановлен с ошибкой", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	pool, err := postgres.New(ctx, cfg.Postgres.DSN())
	if err != nil {
		return err
	}
	defer pool.Close()

	repo := postgres.NewNotificationRepo(pool)
	c := consumer.New(cfg.Kafka.Brokers, cfg.Kafka.Topic, cfg.Kafka.ConsumerGroupID, repo, logger)

	logger.Info("notifier запущен", "topic", cfg.Kafka.Topic, "group", cfg.Kafka.ConsumerGroupID)
	return c.Run(ctx)
}