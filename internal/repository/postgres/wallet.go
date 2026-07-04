// internal/repository/postgres/wallet.go
package postgres

import (
	"bytes"
	"context"
	"errors"
	"fmt"

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

func (r *WalletRepo) Deposit(ctx context.Context, accountID uuid.UUID, amount int64) (domain.Transaction, error) {
	return r.singleEntry(ctx, domain.TxDeposit, accountID, amount, +amount)
}

func (r *WalletRepo) Withdraw(ctx context.Context, accountID uuid.UUID, amount int64) (domain.Transaction, error) {
	return r.singleEntry(ctx, domain.TxWithdraw, accountID, amount, -amount)
}

// singleEntry — deposit/withdraw в одной транзакции с блокировкой строки счёта.
func (r *WalletRepo) singleEntry(
	ctx context.Context,
	txType domain.TransactionType,
	accountID uuid.UUID,
	amount, delta int64,
) (domain.Transaction, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) // no-op после успешного commit

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

	if err := tx.Commit(ctx); err != nil {
		return domain.Transaction{}, fmt.Errorf("commit: %w", err)
	}
	return txn, nil
}

func (r *WalletRepo) Transfer(ctx context.Context, fromID, toID uuid.UUID, amount int64) (domain.Transaction, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// Блокируем счета в детерминированном порядке (по возрастанию id),
	// иначе переводы A->B и B->A могут поймать дедлок.
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
	// Двойная запись: -amount с источника, +amount получателю.
	if err := insertLedger(ctx, tx, txn.ID, fromID, -amount); err != nil {
		return domain.Transaction{}, err
	}
	if err := insertLedger(ctx, tx, txn.ID, toID, +amount); err != nil {
		return domain.Transaction{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Transaction{}, fmt.Errorf("commit: %w", err)
	}
	return txn, nil
}

// lockAccount берёт row-level блокировку (SELECT ... FOR UPDATE) и возвращает баланс.
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