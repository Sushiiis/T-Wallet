package grpcserver

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	walletv1 "github.com/Sushiiis/T-Wallet/api/proto/wallet/v1"
	"github.com/Sushiiis/T-Wallet/internal/auth"
	"github.com/Sushiiis/T-Wallet/internal/ratelimit"
)

// New строит gRPC-сервер с трейсингом, метриками, auth, rate limiting, health-check и reflection.
func New(handler *WalletHandler, tokens *auth.Manager, limiter *ratelimit.Limiter) *grpc.Server {
	srv := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			NewMetricsInterceptor(),
			NewAuthInterceptor(tokens),
			NewRateLimitInterceptor(limiter),
		),
	)

	walletv1.RegisterWalletServiceServer(srv, handler)

	hs := health.NewServer()
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(srv, hs)

	reflection.Register(srv)
	return srv
}