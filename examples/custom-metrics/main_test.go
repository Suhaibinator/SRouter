package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestApplicationUsesCustomMetricsMiddleware(t *testing.T) {
	collector := newRequestMetrics()
	handler, err := newApplication(collector)
	if err != nil {
		t.Fatalf("newApplication() error = %v", err)
	}

	for _, test := range []struct {
		path       string
		wantStatus int
	}{
		{path: "/hello", wantStatus: http.StatusOK},
		{path: "/unavailable", wantStatus: http.StatusServiceUnavailable},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != test.wantStatus {
			t.Fatalf("GET %s status = %d, want %d", test.path, response.Code, test.wantStatus)
		}
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want %d", response.Code, http.StatusOK)
	}

	var snapshot []requestMetric
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	if len(snapshot) != 2 {
		t.Fatalf("metric count = %d, want 2: %#v", len(snapshot), snapshot)
	}

	assertMetric(t, snapshot, "/hello", http.StatusOK)
	assertMetric(t, snapshot, "/unavailable", http.StatusServiceUnavailable)
}

func assertMetric(t *testing.T, snapshot []requestMetric, route string, status int) {
	t.Helper()
	for _, metric := range snapshot {
		if metric.Route == route && metric.Status == status {
			if metric.Requests != 1 {
				t.Fatalf("%s %d requests = %d, want 1", route, status, metric.Requests)
			}
			if metric.TotalDurationSeconds < 0 {
				t.Fatalf("%s %d duration = %f, want non-negative", route, status, metric.TotalDurationSeconds)
			}
			return
		}
	}
	t.Fatalf("missing metric for %s %d in %#v", route, status, snapshot)
}
