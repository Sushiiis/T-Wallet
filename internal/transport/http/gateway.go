package httpserver

import (
	"context"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	walletv1 "github.com/Sushiiis/T-Wallet/api/proto/wallet/v1"
)

// idempotencyHeaderMatcher добавляет проброс "Idempotency-Key" к дефолтным
// правилам grpc-gateway.
func idempotencyHeaderMatcher(key string) (string, bool) {
	switch key {
	case "Idempotency-Key":
		return "idempotency-key", true
	default:
		return runtime.DefaultHeaderMatcher(key)
	}
}

// NewGatewayMux строит REST-шлюз (grpc-gateway), проксирующий JSON HTTP
// на локальный gRPC-сервер по grpcEndpoint.
func NewGatewayMux(ctx context.Context, grpcEndpoint string) (http.Handler, error) {
	mux := runtime.NewServeMux(
		runtime.WithIncomingHeaderMatcher(idempotencyHeaderMatcher),
	)

	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if err := walletv1.RegisterWalletServiceHandlerFromEndpoint(ctx, mux, grpcEndpoint, opts); err != nil {
		return nil, err
	}
	return mux, nil
}