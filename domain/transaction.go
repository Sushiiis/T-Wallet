package domain

import (
	"time"

	"github.com/google/uuid"
)

type TransactionType string

const (
	TxDeposit  TransactionType = "deposit"
	TxWithdraw TransactionType = "withdraw"
	TxTransfer TransactionType = "transfer"
)

const (
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

type Transaction struct {
	ID            uuid.UUID
	Type          TransactionType
	Status        string
	Amount        int64
	FromAccountID *uuid.UUID // nil для deposit
	ToAccountID   *uuid.UUID // nil для withdraw
	CreatedAt     time.Time
}