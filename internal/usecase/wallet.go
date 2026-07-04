package usecase

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/Sushiiis/T-Wallet/domain"
	"github.com/Sushiiis/T-Wallet/internal/auth"
)

// --- интерфейсы (реализация — internal/repository/postgres) ---

type UserRepository interface {
	Create(ctx context.Context, email, passwordHash string) (domain.User, error)
	GetByEmail(ctx context.Context, email string) (domain.User, error)
}

type WalletRepository interface {
	CreateAccount(ctx context.Context, userID uuid.UUID, currency string) (domain.Account, error)
	GetAccount(ctx context.Context, id uuid.UUID) (domain.Account, error)
	Deposit(ctx context.Context, accountID uuid.UUID, amount int64) (domain.Transaction, error)
	Withdraw(ctx context.Context, accountID uuid.UUID, amount int64) (domain.Transaction, error)
	Transfer(ctx context.Context, fromID, toID uuid.UUID, amount int64) (domain.Transaction, error)
}

type TokenManager interface {
	Generate(userID uuid.UUID) (string, error)
}

type Wallet struct {
	users  UserRepository
	wallet WalletRepository
	tokens TokenManager
}

func NewWallet(users UserRepository, wallet WalletRepository, tokens TokenManager) *Wallet {
	return &Wallet{users: users, wallet: wallet, tokens: tokens}
}

// Register создаёт пользователя. Пароль хранится только как bcrypt-хеш.
func (w *Wallet) Register(ctx context.Context, email, password string) (domain.User, error) {
	if email == "" || len(password) < 6 {
		return domain.User{}, domain.ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, err
	}
	return w.users.Create(ctx, email, string(hash))
}

// Login проверяет пароль и выдаёт JWT.
func (w *Wallet) Login(ctx context.Context, email, password string) (string, error) {
	user, err := w.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return "", domain.ErrInvalidCredentials // не раскрываем, что юзера нет
		}
		return "", err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", domain.ErrInvalidCredentials
	}
	return w.tokens.Generate(user.ID)
}

func (w *Wallet) CreateAccount(ctx context.Context, currency string) (domain.Account, error) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		return domain.Account{}, domain.ErrAccessDenied
	}
	if currency == "" {
		currency = "RUB"
	}
	return w.wallet.CreateAccount(ctx, userID, currency)
}

func (w *Wallet) GetBalance(ctx context.Context, accountID uuid.UUID) (domain.Account, error) {
	return w.ownedAccount(ctx, accountID)
}

func (w *Wallet) Deposit(ctx context.Context, accountID uuid.UUID, amount int64) (domain.Transaction, error) {
	if amount <= 0 {
		return domain.Transaction{}, domain.ErrInvalidAmount
	}
	if _, err := w.ownedAccount(ctx, accountID); err != nil {
		return domain.Transaction{}, err
	}
	return w.wallet.Deposit(ctx, accountID, amount)
}

func (w *Wallet) Withdraw(ctx context.Context, accountID uuid.UUID, amount int64) (domain.Transaction, error) {
	if amount <= 0 {
		return domain.Transaction{}, domain.ErrInvalidAmount
	}
	if _, err := w.ownedAccount(ctx, accountID); err != nil {
		return domain.Transaction{}, err
	}
	return w.wallet.Withdraw(ctx, accountID, amount)
}

func (w *Wallet) Transfer(ctx context.Context, fromID, toID uuid.UUID, amount int64) (domain.Transaction, error) {
	if amount <= 0 {
		return domain.Transaction{}, domain.ErrInvalidAmount
	}
	if fromID == toID {
		return domain.Transaction{}, domain.ErrSameAccount
	}
	// Проверяем владельца только для счёта-источника: получатель может быть чужим.
	if _, err := w.ownedAccount(ctx, fromID); err != nil {
		return domain.Transaction{}, err
	}
	return w.wallet.Transfer(ctx, fromID, toID, amount)
}

// ownedAccount загружает счёт и проверяет, что он принадлежит текущему пользователю.
func (w *Wallet) ownedAccount(ctx context.Context, accountID uuid.UUID) (domain.Account, error) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		return domain.Account{}, domain.ErrAccessDenied
	}
	acc, err := w.wallet.GetAccount(ctx, accountID)
	if err != nil {
		return domain.Account{}, err
	}
	if acc.UserID != userID {
		return domain.Account{}, domain.ErrAccessDenied
	}
	return acc, nil
}