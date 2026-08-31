package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	ReconcileTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "inference_operator_reconcile_total",
			Help: "Total number of reconciliations",
		},
	)

	ReconcileErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "inference_operator_reconcile_errors_total",
			Help: "Total number of reconciliation errors",
		},
	)

	ReconcileDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "inference_operator_reconcile_duration_seconds",
			Help:    "Duration of reconciliations in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)
)

func init() {
	metrics.Registry.MustRegister(
		ReconcileTotal,
		ReconcileDuration,
		ReconcileErrors,
	)
}
