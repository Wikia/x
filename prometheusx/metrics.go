package prometheus

import (
	"net/http"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/negroni"
)

// Metrics prototypes
type Metrics struct {
	responseTime    *prometheus.HistogramVec
	totalRequests   *prometheus.CounterVec
	duration        *prometheus.HistogramVec
	responseSize    *prometheus.HistogramVec
	requestSize     *prometheus.HistogramVec
	handlerStatuses *prometheus.CounterVec
}

// Method for creation new custom Prometheus  metrics
func NewMetrics(app, version, hash, date string) *Metrics {
	labels := map[string]string{
		"version":   version,
		"hash":      hash,
		"buildTime": date,
	}
	pm := &Metrics{
		responseTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        app + "_response_time_seconds",
				Help:        "Description",
				ConstLabels: labels,
			},
			[]string{"endpoint"},
		),
		totalRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        app + "_requests_total",
			Help:        "number of requests",
			ConstLabels: labels,
		}, []string{"code", "method"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        app + "_duration_seconds",
			Help:        "duration of a requests in seconds",
			ConstLabels: labels,
		}, []string{"code", "method"}),
		responseSize: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        app + "_response_size_bytes",
			Help:        "size of the responses in bytes",
			ConstLabels: labels,
		}, []string{"code", "method"}),
		requestSize: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        app + "_request_size_bytes",
			Help:        "size of the requests in bytes",
			ConstLabels: labels,
		}, []string{"code", "method"}),
		handlerStatuses: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        app + "_statuses_total",
			Help:        "count number of responses per status",
			ConstLabels: labels,
		}, []string{"method", "status_bucket"}),
	}

	err := prometheus.Register(pm)
	if e := new(prometheus.AlreadyRegisteredError); errors.As(err, e) {
		return pm
	} else if err != nil {
		panic(err)
	}

	return pm
}

// Describe implements prometheus Collector interface.
func (h *Metrics) Describe(in chan<- *prometheus.Desc) {
	h.duration.Describe(in)
	h.totalRequests.Describe(in)
	h.requestSize.Describe(in)
	h.responseSize.Describe(in)
	h.handlerStatuses.Describe(in)
	h.responseTime.Describe(in)
}

// Collect implements prometheus Collector interface.
func (h *Metrics) Collect(in chan<- prometheus.Metric) {
	h.duration.Collect(in)
	h.totalRequests.Collect(in)
	h.requestSize.Collect(in)
	h.responseSize.Collect(in)
	h.handlerStatuses.Collect(in)
	h.responseTime.Collect(in)
}

func (h Metrics) instrumentHandlerStatusBucket(next http.Handler) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(rw, r)

		res, ok := rw.(negroni.ResponseWriter)
		if !ok {
			return
		}

		statusBucket := "unknown"
		switch {
		case res.Status() >= 200 && res.Status() <= 299:
			statusBucket = "2xx"
		case res.Status() >= 300 && res.Status() <= 399:
			statusBucket = "3xx"
		case res.Status() >= 400 && res.Status() <= 499:
			statusBucket = "4xx"
		case res.Status() >= 500 && res.Status() <= 599:
			statusBucket = "5xx"
		}

		h.handlerStatuses.With(prometheus.Labels{"method": r.Method, "status_bucket": statusBucket}).
			Inc()
	}
}

// Instrument will instrument any http.HandlerFunc with custom metrics
func (h Metrics) Instrument(next http.HandlerFunc, endpoint string) http.HandlerFunc {
	wrapped := promhttp.InstrumentHandlerResponseSize(h.responseSize, next)
	wrapped = promhttp.InstrumentHandlerCounter(h.totalRequests, wrapped)
	wrapped = promhttp.InstrumentHandlerDuration(h.duration, wrapped)
	wrapped = promhttp.InstrumentHandlerDuration(h.responseTime.MustCurryWith(prometheus.Labels{"endpoint": endpoint}), wrapped)
	wrapped = promhttp.InstrumentHandlerRequestSize(h.requestSize, wrapped)
	wrapped = h.instrumentHandlerStatusBucket(wrapped)

	return wrapped.ServeHTTP
}
