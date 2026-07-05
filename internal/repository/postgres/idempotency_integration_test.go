// internal/repository/postgres/idempotency_integration_test.go
//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/Sushiiis/T-Wallet/internal/repository/postgres"
)

func TestWalletRepo_Deposit_ConcurrentSameKey_ChargesOnce(t *testing.T) {
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("twallet_test"),
		tcpostgres.WithUsername("twallet"),
		tcpostgres.WithPassword("twallet_pass"),
		tcpostgres.WithInitScripts("../../../migrations/000001_init.up.sql"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	defer func() { _ = testcontainers.TerminateContainer(ctr) }()

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := postgres.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	// FK: accounts.user_id -> users.id, поэтому сначала создаём пользователя.
	userID := uuid.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`,
		userID, "test@example.com", "hash",
	)
	require.NoError(t, err)

	repo := postgres.NewWalletRepo(pool)
	acc, err := repo.CreateAccount(ctx, userID, "RUB")
	require.NoError(t, err)

	const workers = 10
	idempotencyKey := uuid.NewString() // одинаковый ключ у всех горутин
	reqHash := "same-request-hash"

	var wg sync.WaitGroup
	txnIDs := make([]uuid.UUID, workers)
	errs := make([]error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			txn, err := repo.Deposit(ctx, acc.ID, 10_000, idempotencyKey, reqHash)
			errs[i] = err
			if err == nil {
				txnIDs[i] = txn.ID
			}
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "worker %d вернул ошибку", i)
	}
	for i := 1; i < workers; i++ {
		require.Equal(t, txnIDs[0], txnIDs[i], "все горутины должны получить один и тот же transaction_id")
	}

	final, err := repo.GetAccount(ctx, acc.ID)
	require.NoError(t, err)
	require.Equal(t, int64(10_000), final.Balance, "баланс должен измениться только один раз, несмотря на %d параллельных запросов", workers)
}

func TestWalletRepo_Deposit_SameKeyDifferentBody_Conflict(t *testing.T) {
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("twallet_test"),
		tcpostgres.WithUsername("twallet"),
		tcpostgres.WithPassword("twallet_pass"),
		tcpostgres.WithInitScripts("../../../migrations/000001_init.up.sql"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	defer func() { _ = testcontainers.TerminateContainer(ctr) }()

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := postgres.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	userID := uuid.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`,
		userID, "test2@example.com", "hash",
	)
	require.NoError(t, err)

	repo := postgres.NewWalletRepo(pool)
	acc, err := repo.CreateAccount(ctx, userID, "RUB")
	require.NoError(t, err)

	key := uuid.NewString()
	_, err = repo.Deposit(ctx, acc.ID, 10_000, key, "hash-A")
	require.NoError(t, err)

	// Тот же ключ, но другое тело запроса — ожидаем конфликт, а не тихий повтор.
	_, err = repo.Deposit(ctx, acc.ID, 5_000, key, "hash-B")
	require.Error(t, err)

	_ = time.Second // избегаем неиспользуемого импорта, если решите добавить таймауты
}