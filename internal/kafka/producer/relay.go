// internal/kafka/producer/relay.go
package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/segmentio/kafka-go"
)

type outboxRow struct {
	ID      string
	Topic   string
	Payload json.RawMessage
}

// Relay — горутина transactional outbox: читает неопубликованные строки из
// таблицы outbox, публикует в Kafka, помечает published=true только после
// подтверждённой доставки (см. ТЗ п.6.2).
type Relay struct {
	pool     *pgxpool.Pool
	writer   *kafka.Writer
	interval time.Duration
	logger   *slog.Logger
}

func NewRelay(pool *pgxpool.Pool, brokers []string, topic string, interval time.Duration, logger *slog.Logger) *Relay {
	return &Relay{
		pool: pool,
		writer: &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        topic,
			Balancer:     &kafka.LeastBytes{},
			RequiredAcks: kafka.RequireAll,
		},
		interval: interval,
		logger:   logger,
	}
}

// Run блокирует вызывающего до отмены ctx. Предполагается запуск в отдельной горутине.
func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	defer r.writer.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.relayBatch(ctx); err != nil {
				r.logger.Error("outbox relay: ошибка обработки батча", "error", err)
			}
		}
	}
}

func (r *Relay) relayBatch(ctx context.Context) error {
	const selectQ = `
		SELECT id, topic, payload FROM outbox
		WHERE published = false
		ORDER BY created_at
		LIMIT 100`

	rows, err := r.pool.Query(ctx, selectQ)
	if err != nil {
		return fmt.Errorf("select outbox: %w", err)
	}

	var batch []outboxRow
	for rows.Next() {
		var row outboxRow
		if err := rows.Scan(&row.ID, &row.Topic, &row.Payload); err != nil {
			rows.Close()
			return fmt.Errorf("scan outbox row: %w", err)
		}
		batch = append(batch, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate outbox rows: %w", err)
	}
	if len(batch) == 0 {
		return nil
	}

	for _, row := range batch {
		msg := kafka.Message{
			Key:   []byte(row.ID),
			Value: row.Payload,
		}
		// Публикуем по одной записи, чтобы падение одной не блокировало остальные.
		if err := r.writer.WriteMessages(ctx, msg); err != nil {
			r.logger.Error("outbox relay: не удалось опубликовать", "outbox_id", row.ID, "error", err)
			continue // не помечаем published=true — заберём в следующем цикле
		}
		if _, err := r.pool.Exec(ctx,
			`UPDATE outbox SET published = true WHERE id = $1`, row.ID,
		); err != nil {
			r.logger.Error("outbox relay: не удалось пометить published", "outbox_id", row.ID, "error", err)
		}
	}
	return nil
}