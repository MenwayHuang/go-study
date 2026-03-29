package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// TaskPublishedTotal 统计 producer 一共发布了多少条任务。
	TaskPublishedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{Name: "task_published_total", Help: "Total tasks published."},
	)
	// TaskConsumedTotal 按结果维度统计消费结果：
	// - ok: 正常消费成功
	// - error: 处理失败
	// - dedup: 被幂等逻辑识别为重复消息
	TaskConsumedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "task_consumed_total", Help: "Total tasks consumed."},
		[]string{"result"},
	)
	// TaskConsumeDuration 用来观察消息处理时延分布，Grafana 里会基于它算 P95/P99。
	TaskConsumeDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "task_consume_duration_seconds",
			Help:    "Task consume duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
	)
	// TaskRetryTotal 统计“超过最大重试次数后被送入 DLQ”的次数。
	TaskRetryTotal = prometheus.NewCounter(
		prometheus.CounterOpts{Name: "task_retry_total", Help: "Total task retries (sent to DLQ)."},
	)
	// TaskRetryScheduledTotal 统计进入 retry queue 的次数，用来观察延迟重试是否频繁发生。
	TaskRetryScheduledTotal = prometheus.NewCounter(
		prometheus.CounterOpts{Name: "task_retry_scheduled_total", Help: "Total task retries scheduled to retry queue."},
	)
	// TaskDLQTotal 统计最终进入死信队列的消息数量。
	TaskDLQTotal = prometheus.NewCounter(
		prometheus.CounterOpts{Name: "task_dlq_total", Help: "Total tasks sent to dead letter queue."},
	)
	// TaskRateLimitWaitTotal 统计有多少条消息在真正处理前被限流器挡住等待过。
	TaskRateLimitWaitTotal = prometheus.NewCounter(
		prometheus.CounterOpts{Name: "task_rate_limit_wait_total", Help: "Total tasks delayed by consumer rate limiting."},
	)
	// TaskCircuitOpenTotal 统计熔断打开期间，有多少条消息没有直接执行业务处理而是走了重试路径。
	TaskCircuitOpenTotal = prometheus.NewCounter(
		prometheus.CounterOpts{Name: "task_circuit_open_total", Help: "Total tasks blocked because circuit breaker was open."},
	)
	// TaskCircuitState 是熔断器当前状态：1 表示 open，0 表示 closed。
	TaskCircuitState = prometheus.NewGauge(
		prometheus.GaugeOpts{Name: "task_circuit_open", Help: "Circuit breaker state for task consumer (1=open, 0=closed)."},
	)
	// QueueMessages / QueueConsumers 来自 RabbitMQ Management API，反映各队列深度和消费者数。
	QueueMessages = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "rabbitmq_queue_messages", Help: "RabbitMQ queue messages (depth)."},
		[]string{"queue"},
	)
	QueueConsumers = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "rabbitmq_queue_consumers", Help: "RabbitMQ queue consumers."},
		[]string{"queue"},
	)
)

func init() {
	prometheus.MustRegister(TaskPublishedTotal, TaskConsumedTotal, TaskConsumeDuration, TaskRetryTotal, TaskRetryScheduledTotal, TaskDLQTotal, TaskRateLimitWaitTotal, TaskCircuitOpenTotal, TaskCircuitState, QueueMessages, QueueConsumers)
}

func Handler() http.Handler {
	return promhttp.Handler()
}
