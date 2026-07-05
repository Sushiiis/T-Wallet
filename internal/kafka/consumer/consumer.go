// internal/kafka/consumer/consumer.go
package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"

	walletkafka "github.com/Sushiiis/T-Wallet/internal/kafka"
)

type NotificationRepository interface {
	Create(ctx context.Context, transactionID uuid.UUID, payload []byte) (bool, error)
}

type Consumer struct {
	reader *kafka.Reader
	repo   NotificationRepository
	logger *slog.Logger
}

func New(brokers []string, topic, groupID string, repo NotificationRepository, logger *slog.Logger) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		Topic:    topic,
		GroupID:  groupID,
		MinBytes: 1,
		MaxBytes: 10e6,
	})
	return &Consumer{reader: reader, repo: repo, logger: logger}
}

// Run блокирует вызывающего до отмены ctx или фатальной ошибки чтения.
func (c *Consumer) Run(ctx context.Context) error {
	defer c.reader.Close()

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return fmt.Errorf("fetch message: %w", err)
		}

		if err := c.handle(ctx, msg); err != nil {
			// Не коммитим оффсет — сообщение будет доставлено повторно.
			// Обработчик идемпотентен (см. NotificationRepository.Create), это безопасно.
			c.logger.Error("notifier: ошибка обработки сообщения", "error", err)
			continue
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			c.logger.Error("notifier: не удалось закоммитить оффсет", "error", err)
		}
	}
}

func (c *Consumer) handle(ctx context.Context, msg kafka.Message) error {
	var event walletkafka.TransactionCompletedEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		return fmt.Errorf("unmarshal event: %w", err)
	}

	txnID, err := uuid.Parse(event.TransactionID)
	if err != nil {
		return fmt.Errorf("parse transaction_id: %w", err)
	}

	inserted, err := c.repo.Create(ctx, txnID, msg.Value)
	if err != nil {
		return fmt.Errorf("create notification: %w", err)
	}
	if inserted {
		c.logger.Info("notifier: уведомление создано", "transaction_id", event.TransactionID, "type", event.Type)
	} else {
		c.logger.Info("notifier: дубликат события пропущен", "transaction_id", event.TransactionID)
	}
	return nil
}