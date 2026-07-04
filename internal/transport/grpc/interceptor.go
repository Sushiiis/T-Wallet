// internal/transport/grpc/interceptor.go
package grpcserver

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/Sushiiis/T-Wallet/internal/auth"
)

// Методы кошелька, доступные без токена.
var publicMethods = map[string]bool{
	"/wallet.v1.WalletService/Register": true,
	"/wallet.v1.WalletService/Login":    true,
}

// NewAuthInterceptor валидирует JWT и кладёт user_id в контекст.
func NewAuthInterceptor(tokens *auth.Manager) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// Аутентификация только для методов кошелька, кроме Register/Login.
		// health/reflection проходят мимо.
		if !strings.HasPrefix(info.FullMethod, "/wallet.v1.WalletService/") || publicMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}
		values := md.Get("authorization")
		if len(values) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization token")
		}
		token := strings.TrimPrefix(values[0], "Bearer ")

		userID, err := tokens.Parse(token)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}
		return handler(auth.ContextWithUserID(ctx, userID), req)
	}
}