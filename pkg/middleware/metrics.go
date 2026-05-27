package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsCollector holds the Prometheus instruments for HTTP request observability.
type MetricsCollector struct {
	requestsTotal     *prometheus.CounterVec
	requestDuration   *prometheus.HistogramVec
	requestsInFlight  *prometheus.GaugeVec
	responseSizeBytes *prometheus.HistogramVec
}

// NewMetrics creates and registers Prometheus metrics under the given namespace.
func NewMetrics(namespace string) *MetricsCollector {
	return &MetricsCollector{
		requestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests partitioned by method, path, and status.",
			},
			[]string{"method", "path", "status"},
		),
		requestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request latency in seconds.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "path", "status"},
		),
		requestsInFlight: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "http_requests_in_flight",
				Help:      "Current number of in-flight HTTP requests.",
			},
			[]string{"method"},
		),
		responseSizeBytes: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_response_size_bytes",
				Help:      "HTTP response size in bytes.",
				Buckets:   []float64{256, 1024, 4096, 16384, 65536, 262144, 1048576},
			},
			[]string{"method", "path"},
		),
	}
}

// PrometheusHandler returns the default Prometheus metrics scrape endpoint handler.
// Mount this at /metrics.
func PrometheusHandler() http.Handler {
	return promhttp.Handler()
}

// Handler returns an HTTP middleware that records Prometheus metrics.
func (m *MetricsCollector) Handler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			method := r.Method
			path := r.URL.Path

			m.requestsInFlight.WithLabelValues(method).Inc()
			defer m.requestsInFlight.WithLabelValues(method).Dec()

			wrapped := &metricsResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrapped, r)

			duration := time.Since(start).Seconds()
			status := strconv.Itoa(wrapped.statusCode)

			m.requestsTotal.WithLabelValues(method, path, status).Inc()
			m.requestDuration.WithLabelValues(method, path, status).Observe(duration)
			m.responseSizeBytes.WithLabelValues(method, path).Observe(float64(wrapped.bytesWritten))
		})
	}
}

type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
	wroteHeader  bool
}

func (rw *metricsResponseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.statusCode = code
		rw.wroteHeader = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *metricsResponseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}
