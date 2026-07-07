package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Sushiiis/T-Wallet/domain"
)

type WalletRepo struct {
	pool *pgxpool.Pool
}

func NewWalletRepo(pool *pgxpool.Pool) *WalletRepo {
	return &WalletRepo{pool: pool}
}

func (r *WalletRepo) CreateAccount(ctx context.Context, userID uuid.UUID, currency string) (domain.Account, error) {
	const q = `
		INSERT INTO accounts (user_id, currency)
		VALUES ($1, $2)
		RETURNING id, user_id, currency, balance, created_at`

	var a domain.Account
	err := r.pool.QueryRow(ctx, q, userID, currency).
		Scan(&a.ID, &a.UserID, &a.Currency, &a.Balance, &a.CreatedAt)
	if err != nil {
		return domain.Account{}, fmt.Errorf("create account: %w", err)
	}
	return a, nil
}

func (r *WalletRepo) GetAccount(ctx context.Context, id uuid.UUID) (domain.Account, error) {
	const q = `SELECT id, user_id, currency, balance, created_at FROM accounts WHERE id = $1`

	var a domain.Account
	err := r.pool.QueryRow(ctx, q, id).
		Scan(&a.ID, &a.UserID, &a.Currency, &a.Balance, &a.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Account{}, domain.ErrAccountNotFound
		}
		return domain.Account{}, fmt.Errorf("get account: %w", err)
	}
	return a, nil
}

func (r *WalletRepo) Deposit(ctx context.Context, accountID uuid.UUID, amount int64, idempotencyKey, requestHash string) (domain.Transaction, error) {
	return r.singleEntry(ctx, domain.TxDeposit, accountID, amount, +amount, idempotencyKey, requestHash)
}

func (r *WalletRepo) Withdraw(ctx context.Context, accountID uuid.UUID, amount int64, idempotencyKey, requestHash string) (domain.Transaction, error) {
	return r.singleEntry(ctx, domain.TxWithdraw, accountID, amount, -amount, idempotencyKey, requestHash)
}

func (r *WalletRepo) singleEntry(
	ctx context.Context,
	txType domain.TransactionType,
	accountID uuid.UUID,
	amount, delta int64,
	idempotencyKey, reqHash string,
) (domain.Transaction, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	existingTxnID, err := claimIdempotencyKey(ctx, tx, idempotencyKey, reqHash)
	if err != nil {
		return domain.Transaction{}, err
	}
	if existingTxnID != nil {
		txn, err := getTransactionByID(ctx, tx, *existingTxnID)
		if err != nil {
			return domain.Transaction{}, err
		}
		return txn, tx.Commit(ctx)
	}

	balance, err := lockAccount(ctx, tx, accountID)
	if err != nil {
		return domain.Transaction{}, err
	}
	if balance+delta < 0 {
		return domain.Transaction{}, domain.ErrInsufficientFunds
	}

	if _, err := tx.Exec(ctx,
		`UPDATE accounts SET balance = balance + $1 WHERE id = $2`, delta, accountID,
	); err != nil {
		return domain.Transaction{}, fmt.Errorf("update balance: %w", err)
	}

	var fromID, toID *uuid.UUID
	if txType == domain.TxDeposit {
		toID = &accountID
	} else {
		fromID = &accountID
	}

	txn, err := insertTransaction(ctx, tx, txType, amount, fromID, toID)
	if err != nil {
		return domain.Transaction{}, err
	}
	if err := insertLedger(ctx, tx, txn.ID, accountID, delta); err != nil {
		return domain.Transaction{}, err
	}
	if err := attachIdempotencyKey(ctx, tx, idempotencyKey, txn.ID); err != nil {
		return domain.Transaction{}, err
	}
	if err := insertOutboxEvent(ctx, tx, txn); err != nil {
		return domain.Transaction{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Transaction{}, fmt.Errorf("commit: %w", err)
	}
	return txn, nil
}

func (r *WalletRepo) Transfer(ctx context.Context, fromID, toID uuid.UUID, amount int64, idempotencyKey, reqHash string) (domain.Transaction, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	existingTxnID, err := claimIdempotencyKey(ctx, tx, idempotencyKey, reqHash)
	if err != nil {
		return domain.Transaction{}, err
	}
	if existingTxnID != nil {
		txn, err := getTransactionByID(ctx, tx, *existingTxnID)
		if err != nil {
			return domain.Transaction{}, err
		}
		return txn, tx.Commit(ctx)
	}

	firstID, secondID := fromID, toID
	if bytes.Compare(fromID[:], toID[:]) > 0 {
		firstID, secondID = toID, fromID
	}

	balByID := make(map[uuid.UUID]int64, 2)
	for _, id := range []uuid.UUID{firstID, secondID} {
		bal, err := lockAccount(ctx, tx, id)
		if err != nil {
			return domain.Transaction{}, err
		}
		balByID[id] = bal
	}

	if balByID[fromID] < amount {
		return domain.Transaction{}, domain.ErrInsufficientFunds
	}

	if _, err := tx.Exec(ctx,
		`UPDATE accounts SET balance = balance - $1 WHERE id = $2`, amount, fromID,
	); err != nil {
		return domain.Transaction{}, fmt.Errorf("debit: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE accounts SET balance = balance + $1 WHERE id = $2`, amount, toID,
	); err != nil {
		return domain.Transaction{}, fmt.Errorf("credit: %w", err)
	}

	txn, err := insertTransaction(ctx, tx, domain.TxTransfer, amount, &fromID, &toID)
	if err != nil {
		return domain.Transaction{}, err
	}
	if err := insertLedger(ctx, tx, txn.ID, fromID, -amount); err != nil {
		return domain.Transaction{}, err
	}
	if err := insertLedger(ctx, tx, txn.ID, toID, +amount); err != nil {
		return domain.Transaction{}, err
	}
	if err := attachIdempotencyKey(ctx, tx, idempotencyKey, txn.ID); err != nil {
		return domain.Transaction{}, err
	}
	if err := insertOutboxEvent(ctx, tx, txn); err != nil {
		return domain.Transaction{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Transaction{}, fmt.Errorf("commit: %w", err)
	}
	return txn, nil
}

func insertOutboxEvent(ctx context.Context, tx pgx.Tx, txn domain.Transaction) error {
	event := struct {
		TransactionID string     `json:"transaction_id"`
		Type          string     `json:"type"`
		Status        string     `json:"status"`
		Amount        int64      `json:"amount"`
		FromAccountID *uuid.UUID `json:"from_account_id,omitempty"`
		ToAccountID   *uuid.UUID `json:"to_account_id,omitempty"`
		CreatedAt     time.Time  `json:"created_at"`
	}{
		TransactionID: txn.ID.String(),
		Type:          string(txn.Type),
		Status:        txn.Status,
		Amount:        txn.Amount,
		FromAccountID: txn.FromAccountID,
		ToAccountID:   txn.ToAccountID,
		CreatedAt:     txn.CreatedAt,
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal outbox event: %w", err)
	}
	const q = `INSERT INTO outbox (topic, payload) VALUES ($1, $2)`
	if _, err := tx.Exec(ctx, q, "wallet.transactions.completed", payload); err != nil {
		return fmt.Errorf("insert outbox: %w", err)
	}
	return nil
}

func claimIdempotencyKey(ctx context.Context, tx pgx.Tx, key, reqHash string) (*uuid.UUID, error) {
	tag, err := tx.Exec(ctx,
		`INSERT INTO idempotency_keys (key, request_hash) VALUES ($1, $2) ON CONFLICT (key) DO NOTHING`,
		key, reqHash,
	)
	if err != nil {
		return nil, fmt.Errorf("claim idempotency key: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return nil, nil
	}

	var storedHash string
	var existingTxnID *uuid.UUID
	err = tx.QueryRow(ctx,
		`SELECT request_hash, transaction_id FROM idempotency_keys WHERE key = $1`, key,
	).Scan(&storedHash, &existingTxnID)
	if err != nil {
		return nil, fmt.Errorf("read idempotency key: %w", err)
	}
	if storedHash != reqHash {
		return nil, domain.ErrIdempotencyConflict
	}
	if existingTxnID == nil {
		return nil, fmt.Errorf("idempotency key %q claimed but has no transaction yet, retry", key)
	}
	return existingTxnID, nil
}

func attachIdempotencyKey(ctx context.Context, tx pgx.Tx, key string, txnID uuid.UUID) error {
	_, err := tx.Exec(ctx,
		`UPDATE idempotency_keys SET transaction_id = $1 WHERE key = $2`, txnID, key,
	)
	if err != nil {
		return fmt.Errorf("attach idempotency key: %w", err)
	}
	return nil
}

func getTransactionByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (domain.Transaction, error) {
	const q = `
		SELECT id, type, status, amount, from_account_id, to_account_id, created_at
		FROM transactions WHERE id = $1`

	var t domain.Transaction
	err := tx.QueryRow(ctx, q, id).
		Scan(&t.ID, &t.Type, &t.Status, &t.Amount, &t.FromAccountID, &t.ToAccountID, &t.CreatedAt)
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("get transaction by id: %w", err)
	}
	return t, nil
}

func lockAccount(ctx context.Context, tx pgx.Tx, id uuid.UUID) (int64, error) {
	var balance int64
	err := tx.QueryRow(ctx,
		`SELECT balance FROM accounts WHERE id = $1 FOR UPDATE`, id,
	).Scan(&balance)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrAccountNotFound
		}
		return 0, fmt.Errorf("lock account: %w", err)
	}
	return balance, nil
}

func insertTransaction(
	ctx context.Context, tx pgx.Tx,
	txType domain.TransactionType, amount int64, fromID, toID *uuid.UUID,
) (domain.Transaction, error) {
	const q = `
		INSERT INTO transactions (type, status, amount, from_account_id, to_account_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, type, status, amount, from_account_id, to_account_id, created_at`

	var t domain.Transaction
	err := tx.QueryRow(ctx, q, txType, domain.StatusCompleted, amount, fromID, toID).
		Scan(&t.ID, &t.Type, &t.Status, &t.Amount, &t.FromAccountID, &t.ToAccountID, &t.CreatedAt)
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("insert transaction: %w", err)
	}
	return t, nil
}

func insertLedger(ctx context.Context, tx pgx.Tx, txID, accountID uuid.UUID, amount int64) error {
	const q = `INSERT INTO ledger_entries (transaction_id, account_id, amount) VALUES ($1, $2, $3)`
	if _, err := tx.Exec(ctx, q, txID, accountID, amount); err != nil {
		return fmt.Errorf("insert ledger: %w", err)
	}
	return nil
}