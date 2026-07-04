// internal/transport/grpc/server.go
package grpcserver

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	walletv1 "github.com/Sushiiis/T-Wallet/api/proto/wallet/v1"
	"github.com/Sushiiis/T-Wallet/internal/auth"
)

// New строит gRPC-сервер с auth-интерсептором, сервисом кошелька,
// health-check и reflection.
func New(handler *WalletHandler, tokens *auth.Manager) *grpc.Server {
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(NewAuthInterceptor(tokens)),
	)

	walletv1.RegisterWalletServiceServer(srv, handler)

	hs := health.NewServer()
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(srv, hs)

	reflection.Register(srv)
	return srv
}