package metrics

import (
	"net/http"
	"strings"
	"testing"
)

// TestHandlerAppliesDefaultTags verifies that DefaultTags from the middleware
// config are applied to every metric the Handler emits, that tags with empty
// values are skipped, and that route/status tags are still added on top.
func TestHandlerAppliesDefaultTags(t *testing.T) {
	registry := NewMockMetricsRegistry()
	middleware := NewMetricsMiddleware[string, any](registry, MetricsMiddlewareConfig{
		EnableLatency:    true,
		EnableThroughput: true,
		EnableQPS:        true,
		EnableErrors:     true,
		DefaultTags: Tags{
			"service": "api",
			"region":  "", // Empty values must not be emitted as tags.
		},
	})

	handler := middleware.Handler("fallback", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req, err := http.NewRequest("POST", "/missing", strings.NewReader("payload"))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	handler.ServeHTTP(NewMockResponseWriter(), req)

	checkTags := func(name string, tags Tags, extra Tags) {
		t.Helper()
		if tags["service"] != "api" {
			t.Errorf("%s: expected default tag service=api, got tags %v", name, tags)
		}
		if _, present := tags["region"]; present {
			t.Errorf("%s: empty-valued default tag %q must not be emitted, got tags %v", name, "region", tags)
		}
		for k, v := range extra {
			if tags[k] != v {
				t.Errorf("%s: expected tag %s=%s, got tags %v", name, k, v, tags)
			}
		}
	}

	latency, ok := registry.histograms["request_latency_seconds"].(*MockHistogram)
	if !ok {
		t.Fatal("expected request_latency_seconds histogram to be built")
	}
	checkTags("request_latency_seconds", latency.Tags(), Tags{"route": "fallback"})

	allLatency, ok := registry.histograms["all_request_latency_seconds"].(*MockHistogram)
	if !ok {
		t.Fatal("expected all_request_latency_seconds histogram to be built")
	}
	checkTags("all_request_latency_seconds", allLatency.Tags(), nil)

	qps, ok := registry.counters["requests_total"].(*MockCounter)
	if !ok {
		t.Fatal("expected requests_total counter to be built")
	}
	checkTags("requests_total", qps.Tags(), Tags{"route": "fallback"})

	throughput, ok := registry.counters["request_throughput_bytes"].(*MockCounter)
	if !ok {
		t.Fatal("expected request_throughput_bytes counter to be built")
	}
	checkTags("request_throughput_bytes", throughput.Tags(), Tags{"route": "fallback"})

	errors, ok := registry.counters["request_errors_total"].(*MockCounter)
	if !ok {
		t.Fatal("expected request_errors_total counter to be built")
	}
	checkTags("request_errors_total", errors.Tags(), Tags{"route": "fallback", "status_code": "404"})
}

// TestHandlerReusesCachedMetrics verifies that the per-route metric instances
// are built once and reused across requests: repeated requests must accumulate
// into the same counter/histogram rather than rebuilding fresh metrics.
func TestHandlerReusesCachedMetrics(t *testing.T) {
	registry := NewMockMetricsRegistry()
	middleware := NewMetricsMiddleware[string, any](registry, MetricsMiddlewareConfig{
		EnableLatency:    true,
		EnableThroughput: true,
		EnableQPS:        true,
	})

	handler := middleware.Handler("cached-route", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	const requests = 3
	const bodySize = 7 // len("payload")
	for range requests {
		req, err := http.NewRequest("POST", "/cached", strings.NewReader("payload"))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		handler.ServeHTTP(NewMockResponseWriter(), req)
	}

	// If caching failed, each request would build (and overwrite) a fresh
	// counter holding only that request's increment.
	qps, ok := registry.counters["requests_total"].(*MockCounter)
	if !ok {
		t.Fatal("expected requests_total counter to be built")
	}
	if qps.Value() != requests {
		t.Errorf("expected requests_total to accumulate %d increments on one instance, got %v", requests, qps.Value())
	}

	totalQPS, ok := registry.counters["all_requests_total"].(*MockCounter)
	if !ok {
		t.Fatal("expected all_requests_total counter to be built")
	}
	if totalQPS.Value() != requests {
		t.Errorf("expected all_requests_total to accumulate %d increments on one instance, got %v", requests, totalQPS.Value())
	}

	throughput, ok := registry.counters["request_throughput_bytes"].(*MockCounter)
	if !ok {
		t.Fatal("expected request_throughput_bytes counter to be built")
	}
	if throughput.Value() != requests*bodySize {
		t.Errorf("expected request_throughput_bytes to accumulate %d bytes on one instance, got %v", requests*bodySize, throughput.Value())
	}

	latency, ok := registry.histograms["request_latency_seconds"].(*MockHistogram)
	if !ok {
		t.Fatal("expected request_latency_seconds histogram to be built")
	}
	if len(latency.observations) != requests {
		t.Errorf("expected %d latency observations on one histogram instance, got %d", requests, len(latency.observations))
	}
}
