package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	TaskPublishedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{Name: "task_published_total", Help: "Total tasks published."},
	)
	TaskConsumedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "task_consumed_total", Help: "Total tasks consumed."},
		[]string{"result"},
	)
	TaskConsumeDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "task_consume_duration_seconds",
			Help:    "Task consume duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
	)
	TaskRetryTotal = prometheus.NewCounter(
		prometheus.CounterOpts{Name: "task_retry_total", Help: "Total task retries (sent to DLQ)."},
	)
)

func init() {
	prometheus.MustRegister(TaskPublishedTotal, TaskConsumedTotal, TaskConsumeDuration, TaskRetryTotal)
}

func Handler() http.Handler {
	return promhttp.Handler()
}
