package domain

import (
	"time"

	"github.com/google/uuid"
)

type Account struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Currency  string
	Balance   int64
	CreatedAt time.Time
}