// internal/observability/metrics.go
package observability

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "wallet_requests_total",
		Help: "Общее число gRPC-запросов по методам и статусам.",
	}, []string{"method", "code"})

	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "wallet_request_duration_seconds",
		Help:    "Латентность gRPC-запросов.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method"})

	OutboxUnpublished = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "wallet_outbox_unpublished",
		Help: "Количество неопубликованных записей в outbox на момент последнего опроса.",
	})
)

// ObserveRequest — вспомогательная функция для интерсептора.
func ObserveRequest(method, code string, duration time.Duration) {
	RequestsTotal.WithLabelValues(method, code).Inc()
	RequestDuration.WithLabelValues(method).Observe(duration.Seconds())
}