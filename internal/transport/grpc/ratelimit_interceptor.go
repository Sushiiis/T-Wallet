package grpcserver

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Sushiiis/T-Wallet/internal/auth"
	"github.com/Sushiiis/T-Wallet/internal/ratelimit"
)

var rateLimitedMethods = map[string]bool{
	"/wallet.v1.WalletService/Deposit":  true,
	"/wallet.v1.WalletService/Withdraw": true,
	"/wallet.v1.WalletService/Transfer": true,
}

func NewRateLimitInterceptor(limiter *ratelimit.Limiter) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !rateLimitedMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		userID, ok := auth.UserIDFromContext(ctx)
		if !ok {
			return handler(ctx, req)
		}

		allowed, err := limiter.Allow(ctx, userID.String())
		if err != nil {
			return handler(ctx, req)
		}
		if !allowed {
			return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded, try again later")
		}
		return handler(ctx, req)
	}
}