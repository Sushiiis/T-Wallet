package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Cтроит сервер: liveness/readiness, всё остальное — в REST-шлюз.
func New(ctx context.Context, addr string, pool *pgxpool.Pool, grpcEndpoint string) (*http.Server, error) {
	gateway, err := NewGatewayMux(ctx, grpcEndpoint)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	
	mux.Handle("GET /metrics", promhttp.Handler())

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	// Всё остальное (/v1/...) — в grpc-gateway.
	// Проверить
	mux.Handle("/v1/", gateway)

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}, nil
}