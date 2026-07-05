// internal/transport/grpc/wallet.go
package grpcserver

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	walletv1 "github.com/Sushiiis/T-Wallet/api/proto/wallet/v1"
	"github.com/Sushiiis/T-Wallet/domain"
	"github.com/Sushiiis/T-Wallet/internal/usecase"
)

type WalletHandler struct {
	walletv1.UnimplementedWalletServiceServer
	uc *usecase.Wallet
}

func NewWalletHandler(uc *usecase.Wallet) *WalletHandler {
	return &WalletHandler{uc: uc}
}

func (h *WalletHandler) Register(ctx context.Context, req *walletv1.RegisterRequest) (*walletv1.RegisterResponse, error) {
	u, err := h.uc.Register(ctx, req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, toStatus(err)
	}
	return &walletv1.RegisterResponse{UserId: u.ID.String()}, nil
}

func (h *WalletHandler) Login(ctx context.Context, req *walletv1.LoginRequest) (*walletv1.LoginResponse, error) {
	token, err := h.uc.Login(ctx, req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, toStatus(err)
	}
	return &walletv1.LoginResponse{AccessToken: token}, nil
}

func (h *WalletHandler) CreateAccount(ctx context.Context, req *walletv1.CreateAccountRequest) (*walletv1.Account, error) {
	acc, err := h.uc.CreateAccount(ctx, req.GetCurrency())
	if err != nil {
		return nil, toStatus(err)
	}
	return &walletv1.Account{
		Id: acc.ID.String(), UserId: acc.UserID.String(),
		Currency: acc.Currency, Balance: acc.Balance,
	}, nil
}

func (h *WalletHandler) GetBalance(ctx context.Context, req *walletv1.GetBalanceRequest) (*walletv1.BalanceResponse, error) {
	id, err := uuid.Parse(req.GetAccountId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid account_id")
	}
	acc, err := h.uc.GetBalance(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &walletv1.BalanceResponse{AccountId: acc.ID.String(), Balance: acc.Balance, Currency: acc.Currency}, nil
}

func (h *WalletHandler) Deposit(ctx context.Context, req *walletv1.DepositRequest) (*walletv1.TransactionResponse, error) {
	id, err := uuid.Parse(req.GetAccountId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid account_id")
	}
	key, err := idempotencyKeyFromMetadata(ctx)
	if err != nil {
		return nil, err
	}
	txn, err := h.uc.Deposit(ctx, id, req.GetAmount(), key)
	if err != nil {
		return nil, toStatus(err)
	}
	return txnResponse(txn), nil
}

func (h *WalletHandler) Withdraw(ctx context.Context, req *walletv1.WithdrawRequest) (*walletv1.TransactionResponse, error) {
	id, err := uuid.Parse(req.GetAccountId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid account_id")
	}
	key, err := idempotencyKeyFromMetadata(ctx)
	if err != nil {
		return nil, err
	}
	txn, err := h.uc.Withdraw(ctx, id, req.GetAmount(), key)
	if err != nil {
		return nil, toStatus(err)
	}
	return txnResponse(txn), nil
}

func (h *WalletHandler) Transfer(ctx context.Context, req *walletv1.TransferRequest) (*walletv1.TransactionResponse, error) {
	fromID, err := uuid.Parse(req.GetFromAccountId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid from_account_id")
	}
	toID, err := uuid.Parse(req.GetToAccountId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid to_account_id")
	}
	key, err := idempotencyKeyFromMetadata(ctx)
	if err != nil {
		return nil, err
	}
	txn, err := h.uc.Transfer(ctx, fromID, toID, req.GetAmount(), key)
	if err != nil {
		return nil, toStatus(err)
	}
	return txnResponse(txn), nil
}

// idempotencyKeyFromMetadata читает заголовок "idempotency-key" из gRPC-метаданных.
// gRPC автоматически приводит имена заголовков к нижнему регистру.
func idempotencyKeyFromMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.InvalidArgument, "missing idempotency-key metadata")
	}
	values := md.Get("idempotency-key")
	if len(values) == 0 || values[0] == "" {
		return "", status.Error(codes.InvalidArgument, "missing idempotency-key metadata")
	}
	return values[0], nil
}

func txnResponse(t domain.Transaction) *walletv1.TransactionResponse {
	return &walletv1.TransactionResponse{
		TransactionId: t.ID.String(), Type: string(t.Type),
		Status: t.Status, Amount: t.Amount,
	}
}

func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrUserAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrIdempotencyConflict):
		return status.Error(codes.AlreadyExists, err.Error()) // grpc-gateway маппит на HTTP 409
	case errors.Is(err, domain.ErrInvalidCredentials):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, domain.ErrAccessDenied):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, domain.ErrAccountNotFound), errors.Is(err, domain.ErrUserNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrInsufficientFunds):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrInvalidAmount), errors.Is(err, domain.ErrSameAccount), errors.Is(err, domain.ErrIdempotencyKeyRequired):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}