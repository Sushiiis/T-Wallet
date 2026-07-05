// internal/repository/postgres/notification.go
package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type NotificationRepo struct {
	pool *pgxpool.Pool
}

func NewNotificationRepo(pool *pgxpool.Pool) *NotificationRepo {
	return &NotificationRepo{pool: pool}
}

// Create идемпотентно записывает уведомление: повторная доставка того же
// transaction_id (at-least-once от Kafka) не создаёт вторую запись.
// Возвращает true, если запись была реально вставлена (не дубликат).
func (r *NotificationRepo) Create(ctx context.Context, transactionID uuid.UUID, payload []byte) (bool, error) {
	const q = `
		INSERT INTO notifications (transaction_id, payload)
		VALUES ($1, $2)
		ON CONFLICT (transaction_id) DO NOTHING`

	tag, err := r.pool.Exec(ctx, q, transactionID, payload)
	if err != nil {
		return false, fmt.Errorf("insert notification: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}