package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/Sushiiis/T-Wallet/domain"
	"github.com/Sushiiis/T-Wallet/internal/auth"
	"github.com/Sushiiis/T-Wallet/internal/usecase"
)

type fakeUsers struct{ byEmail map[string]domain.User }

func (f *fakeUsers) Create(_ context.Context, email, hash string) (domain.User, error) {
	if _, ok := f.byEmail[email]; ok {
		return domain.User{}, domain.ErrUserAlreadyExists
	}
	u := domain.User{ID: uuid.New(), Email: email, PasswordHash: hash}
	f.byEmail[email] = u
	return u, nil
}
func (f *fakeUsers) GetByEmail(_ context.Context, email string) (domain.User, error) {
	u, ok := f.byEmail[email]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

type fakeWallet struct {
	accounts       map[uuid.UUID]domain.Account
	transferCalled bool
	usedKeys       map[string]bool // имитирует БД-уникальность ключа
}

func (f *fakeWallet) CreateAccount(_ context.Context, userID uuid.UUID, cur string) (domain.Account, error) {
	a := domain.Account{ID: uuid.New(), UserID: userID, Currency: cur}
	f.accounts[a.ID] = a
	return a, nil
}
func (f *fakeWallet) GetAccount(_ context.Context, id uuid.UUID) (domain.Account, error) {
	a, ok := f.accounts[id]
	if !ok {
		return domain.Account{}, domain.ErrAccountNotFound
	}
	return a, nil
}
func (f *fakeWallet) Deposit(_ context.Context, _ uuid.UUID, amount int64, key, _ string) (domain.Transaction, error) {
	if f.usedKeys[key] {
		return domain.Transaction{ID: uuid.New(), Type: domain.TxDeposit, Amount: amount}, nil // имитация replay
	}
	f.usedKeys[key] = true
	return domain.Transaction{ID: uuid.New(), Type: domain.TxDeposit, Amount: amount}, nil
}
func (f *fakeWallet) Withdraw(_ context.Context, _ uuid.UUID, amount int64, _, _ string) (domain.Transaction, error) {
	return domain.Transaction{ID: uuid.New(), Type: domain.TxWithdraw, Amount: amount}, nil
}
func (f *fakeWallet) Transfer(_ context.Context, _, _ uuid.UUID, amount int64, _, _ string) (domain.Transaction, error) {
	f.transferCalled = true
	return domain.Transaction{ID: uuid.New(), Type: domain.TxTransfer, Amount: amount}, nil
}

type fakeTokens struct{}

func (fakeTokens) Generate(id uuid.UUID) (string, error) { return "token-" + id.String(), nil }

func newWallet() (*usecase.Wallet, *fakeWallet) {
	users := &fakeUsers{byEmail: map[string]domain.User{}}
	wallet := &fakeWallet{accounts: map[uuid.UUID]domain.Account{}, usedKeys: map[string]bool{}}
	return usecase.NewWallet(users, wallet, fakeTokens{}), wallet
}

func TestRegister_Success(t *testing.T) {
	uc, _ := newWallet()
	u, err := uc.Register(context.Background(), "a@b.c", "secret123")
	require.NoError(t, err)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("secret123")))
}

func TestRegister_Duplicate(t *testing.T) {
	uc, _ := newWallet()
	_, err := uc.Register(context.Background(), "a@b.c", "secret123")
	require.NoError(t, err)
	_, err = uc.Register(context.Background(), "a@b.c", "secret123")
	require.ErrorIs(t, err, domain.ErrUserAlreadyExists)
}

func TestLogin_WrongPassword(t *testing.T) {
	uc, _ := newWallet()
	_, _ = uc.Register(context.Background(), "a@b.c", "secret123")
	_, err := uc.Login(context.Background(), "a@b.c", "wrong")
	require.ErrorIs(t, err, domain.ErrInvalidCredentials)
}

func TestTransfer_SameAccount(t *testing.T) {
	uc, wallet := newWallet()
	userID := uuid.New()
	acc, _ := wallet.CreateAccount(context.Background(), userID, "RUB")
	ctx := auth.ContextWithUserID(context.Background(), userID)
	_, err := uc.Transfer(ctx, acc.ID, acc.ID, 100, "key-1")
	require.ErrorIs(t, err, domain.ErrSameAccount)
	require.False(t, wallet.transferCalled)
}

func TestTransfer_NotOwner(t *testing.T) {
	uc, wallet := newWallet()
	acc, _ := wallet.CreateAccount(context.Background(), uuid.New(), "RUB")
	dst, _ := wallet.CreateAccount(context.Background(), uuid.New(), "RUB")
	ctx := auth.ContextWithUserID(context.Background(), uuid.New())
	_, err := uc.Transfer(ctx, acc.ID, dst.ID, 100, "key-1")
	require.ErrorIs(t, err, domain.ErrAccessDenied)
}

func TestDeposit_InvalidAmount(t *testing.T) {
	uc, wallet := newWallet()
	userID := uuid.New()
	acc, _ := wallet.CreateAccount(context.Background(), userID, "RUB")
	ctx := auth.ContextWithUserID(context.Background(), userID)
	_, err := uc.Deposit(ctx, acc.ID, -50, "key-1")
	require.ErrorIs(t, err, domain.ErrInvalidAmount)
}

func TestDeposit_MissingIdempotencyKey(t *testing.T) {
	uc, wallet := newWallet()
	userID := uuid.New()
	acc, _ := wallet.CreateAccount(context.Background(), userID, "RUB")
	ctx := auth.ContextWithUserID(context.Background(), userID)
	_, err := uc.Deposit(ctx, acc.ID, 100, "")
	require.ErrorIs(t, err, domain.ErrIdempotencyKeyRequired)
}