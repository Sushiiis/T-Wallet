package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/Sushiiis/T-Wallet/internal/repository/postgres"
)

func TestNotificationRepo_Create_DuplicateTransactionID_Idempotent(t *testing.T) {
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
		userID, "notif@test.com", "hash")
	require.NoError(t, err)
	var txnID uuid.UUID
	err = pool.QueryRow(ctx,
		`INSERT INTO transactions (type, status, amount) VALUES ('deposit','completed',100) RETURNING id`,
	).Scan(&txnID)
	require.NoError(t, err)

	repo := postgres.NewNotificationRepo(pool)

	inserted1, err := repo.Create(ctx, txnID, []byte(`{"type":"deposit"}`))
	require.NoError(t, err)
	require.True(t, inserted1)

	inserted2, err := repo.Create(ctx, txnID, []byte(`{"type":"deposit"}`))
	require.NoError(t, err)
	require.False(t, inserted2, "повторная доставка не должна создавать вторую запись")

	var count int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE transaction_id = $1`, txnID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "в таблице должна остаться ровно одна запись, несмотря на два вызова Create")
}