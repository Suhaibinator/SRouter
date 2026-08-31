package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/metrics"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
)

type metricKey struct {
	route  string
	status int
}

type metricValue struct {
	requests      uint64
	totalDuration time.Duration
}

type requestMetric struct {
	Route                string  `json:"route"`
	Status               int     `json:"status"`
	Requests             uint64  `json:"requests"`
	TotalDurationSeconds float64 `json:"total_duration_seconds"`
}

// requestMetrics is a small custom metrics backend. It implements SRouter's
// MetricsMiddleware interface directly and exposes its data as JSON.
type requestMetrics struct {
	mu      sync.RWMutex
	values  map[metricKey]metricValue
	filter  metrics.MetricsFilter
	sampler metrics.MetricsSampler
}

var _ metrics.MetricsMiddleware[string, struct{}] = (*requestMetrics)(nil)

func newRequestMetrics() *requestMetrics {
	return &requestMetrics{values: make(map[metricKey]metricValue)}
}

func (m *requestMetrics) Handler(fallbackName string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.filter != nil && !m.filter.Filter(r) {
			next.ServeHTTP(w, r)
			return
		}
		if m.sampler != nil && !m.sampler.Sample() {
			next.ServeHTTP(w, r)
			return
		}

		started := time.Now()
		writer := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(writer, r)

		routeName, ok := scontext.GetRouteTemplateFromRequest(r)
		if !ok {
			routeName = fallbackName
		}

		key := metricKey{route: routeName, status: writer.status}
		m.mu.Lock()
		value := m.values[key]
		value.requests++
		value.totalDuration += time.Since(started)
		m.values[key] = value
		m.mu.Unlock()
	})
}

// Configure is part of metrics.MetricsMiddleware. This custom backend has no
// built-in feature flags, so it keeps its existing behavior.
func (m *requestMetrics) Configure(_ metrics.MetricsMiddlewareConfig) metrics.MetricsMiddleware[string, struct{}] {
	return m
}

func (m *requestMetrics) WithFilter(filter metrics.MetricsFilter) metrics.MetricsMiddleware[string, struct{}] {
	m.filter = filter
	return m
}

func (m *requestMetrics) WithSampler(sampler metrics.MetricsSampler) metrics.MetricsMiddleware[string, struct{}] {
	m.sampler = sampler
	return m
}

func (m *requestMetrics) snapshot() []requestMetric {
	m.mu.RLock()
	result := make([]requestMetric, 0, len(m.values))
	for key, value := range m.values {
		result = append(result, requestMetric{
			Route:                key.route,
			Status:               key.status,
			Requests:             value.requests,
			TotalDurationSeconds: value.totalDuration.Seconds(),
		})
	}
	m.mu.RUnlock()

	sort.Slice(result, func(i, j int) bool {
		if result[i].Route == result[j].Route {
			return result[i].Status < result[j].Status
		}
		return result[i].Route < result[j].Route
	})
	return result
}

func (m *requestMetrics) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(m.snapshot()); err != nil {
		http.Error(w, "could not encode metrics", http.StatusInternalServerError)
	}
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (w *statusWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func newApplication(requestMetrics *requestMetrics) (http.Handler, error) {
	if requestMetrics == nil {
		return nil, errors.New("request metrics must not be nil")
	}

	r := router.NewRouter[string, struct{}](router.RouterConfig{
		ServiceName: "custom-metrics-example",
		MetricsConfig: &router.MetricsConfig{
			MiddlewareFactory: requestMetrics,
		},
	}, nil, nil)

	r.Route(
		router.RouteConfigBase{
			Path:    "/hello",
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_, _ = fmt.Fprintln(w, "hello")
			},
		},
		router.RouteConfigBase{
			Path:    "/unavailable",
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			},
		},
	)

	if err := r.Build(); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", requestMetrics)
	mux.Handle("/", r)
	return mux, nil
}

func main() {
	var port int
	flag.IntVar(&port, "port", 8080, "port to listen on")
	flag.Parse()

	handler, err := newApplication(newRequestMetrics())
	if err != nil {
		log.Fatal(err)
	}

	address := ":" + strconv.Itoa(port)
	log.Printf("listening on http://localhost%s", address)
	log.Fatal(http.ListenAndServe(address, handler))
}
