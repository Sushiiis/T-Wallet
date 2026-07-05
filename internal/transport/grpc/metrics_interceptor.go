// internal/transport/grpc/metrics_interceptor.go
package grpcserver

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/Sushiiis/T-Wallet/internal/observability"
)

// NewMetricsInterceptor пишет счётчик запросов и гистограмму латентности в Prometheus.
func NewMetricsInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		code := status.Code(err).String()
		observability.ObserveRequest(info.FullMethod, code, time.Since(start))
		return resp, err
	}
}