// internal/repository/postgres/concurrency_integration_test.go
//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/Sushiiis/T-Wallet/internal/repository/postgres"
)

// TestWalletRepo_ConcurrentTransfers_NoNegativeBalance_NoLostMoney — тест
// "на собеседование-эффект" из ТЗ: 100 параллельных переводов между двумя
// счетами не должны увести баланс в минус и не должны "потерять" деньги.
func TestWalletRepo_ConcurrentTransfers_NoNegativeBalance_NoLostMoney(t *testing.T) {
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("twallet_test"),
		tcpostgres.WithUsername("twallet"),
		tcpostgres.WithPassword("twallet_pass"),
		tcpostgres.WithInitScripts(
			"../../../migrations/000001_init.up.sql",
			"../../../migrations/000002_notifications.up.sql",
		),
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
	_, err = pool.Exec(ctx, `INSERT INTO users (id, email, password_hash) VALUES ($1,$2,$3)`,
		userID, "concurrency@test.com", "hash")
	require.NoError(t, err)

	repo := postgres.NewWalletRepo(pool)
	accA, err := repo.CreateAccount(ctx, userID, "RUB")
	require.NoError(t, err)
	accB, err := repo.CreateAccount(ctx, userID, "RUB")
	require.NoError(t, err)

	const (
		workers      = 100
		transferUnit = 1
		initialA     = 10_000
	)
	_, err = repo.Deposit(ctx, accA.ID, initialA, uuid.NewString(), "seed")
	require.NoError(t, err)

	var wg sync.WaitGroup
	var succeeded int64

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Половина переводит A->B, половина B->A — специально бьём в обе
			// стороны, чтобы задействовать и проверить порядок блокировок
			// (см. bytes.Compare в Transfer), а не только один путь.
			from, to := accA.ID, accB.ID
			if i%2 == 1 {
				from, to = accB.ID, accA.ID
			}
			key := uuid.NewString()
			_, err := repo.Transfer(ctx, from, to, transferUnit, key, "concurrent-test")
			if err == nil {
				atomic.AddInt64(&succeeded, 1)
			}
		}(i)
	}
	wg.Wait()

	finalA, err := repo.GetAccount(ctx, accA.ID)
	require.NoError(t, err)
	finalB, err := repo.GetAccount(ctx, accB.ID)
	require.NoError(t, err)

	require.GreaterOrEqual(t, finalA.Balance, int64(0), "баланс A не должен уйти в минус")
	require.GreaterOrEqual(t, finalB.Balance, int64(0), "баланс B не должен уйти в минус")
	require.Equal(t, int64(initialA), finalA.Balance+finalB.Balance,
		"сумма балансов должна сохраниться неизменной несмотря на %d параллельных переводов", workers)
}