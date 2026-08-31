package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApplicationExportsSRouterMetricsThroughPrometheusAdapter(t *testing.T) {
	handler, err := newApplication()
	if err != nil {
		t.Fatalf("newApplication() error = %v", err)
	}

	for _, test := range []struct {
		path       string
		wantStatus int
	}{
		{path: "/api/hello", wantStatus: http.StatusOK},
		{path: "/api/error", wantStatus: http.StatusInternalServerError},
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
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read metrics response: %v", err)
	}

	exposition := string(body)
	for _, want := range []string{
		`example_api_requests_total{route="/api/hello"} 1`,
		`example_api_request_errors_total{route="/api/error",status_code="500"} 1`,
		`example_api_request_latency_seconds_count{route="/api/hello"} 1`,
	} {
		if !strings.Contains(exposition, want) {
			t.Errorf("metrics output does not contain %q\n%s", want, exposition)
		}
	}
}
