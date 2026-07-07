package domain

import "errors"

var (
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountNotFound    = errors.New("account not found")
	ErrAccessDenied       = errors.New("access denied")
	ErrInsufficientFunds  = errors.New("insufficient funds")
	ErrInvalidAmount      = errors.New("amount must be positive")
	ErrSameAccount        = errors.New("cannot transfer to the same account")
	ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
	ErrIdempotencyConflict    = errors.New("idempotency key used with a different request")
)