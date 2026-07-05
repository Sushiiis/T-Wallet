// internal/kafka/event.go
package kafka

import "time"

// TransactionCompletedEvent — payload события, которое relay публикует в Kafka
// и которое консьюмер notifier'а десериализует обратно.
type TransactionCompletedEvent struct {
	TransactionID string    `json:"transaction_id"`
	Type          string    `json:"type"`
	Status        string    `json:"status"`
	Amount        int64     `json:"amount"`
	FromAccountID *string   `json:"from_account_id,omitempty"`
	ToAccountID   *string   `json:"to_account_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}