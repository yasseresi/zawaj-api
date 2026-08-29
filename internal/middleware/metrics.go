package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// RED metrics for HTTP: request Rate, Errors (via status), Duration. Labels use
// the gin route template (c.FullPath()), not the raw path, so per-id URLs do not
// explode label cardinality.
var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests processed, by method, route, and status.",
		},
		[]string{"method", "route", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds, by method and route.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route"},
	)

	httpRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "In-flight HTTP requests.",
		},
	)
)

// Metrics records Prometheus RED metrics for every request. Requests to
// unmatched routes (404) are grouped under the "unmatched" route label to keep
// cardinality bounded.
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		httpRequestsInFlight.Inc()

		c.Next()

		httpRequestsInFlight.Dec()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		method := c.Request.Method
		httpRequestDuration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
		httpRequestsTotal.WithLabelValues(method, route, strconv.Itoa(c.Writer.Status())).Inc()
	}
}
