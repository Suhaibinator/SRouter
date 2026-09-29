package router

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/logkeys"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestHijackedRequestSummaryIsWrittenAtHandoff covers long-lived WebSocket
// connections: the summary is written once when the handler takes over the
// connection, not when the connection finally closes.
func TestHijackedRequestSummaryIsWrittenAtHandoff(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	r := NewRouter(RouterConfig{
		Logger:             zap.New(core),
		EnableTraceLogging: true,
	}, RouterDependencies[string, struct{}]{})

	release := make(chan struct{})
	r.Route(RouteConfigBase{
		Path:    "/ws",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			conn, brw, err := http.NewResponseController(w).Hijack()
			if err != nil {
				t.Errorf("Hijack() error = %v", err)
				return
			}
			defer func() { _ = conn.Close() }()
			_, _ = brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: test\r\n\r\n")
			_ = brw.Flush()
			<-release
		},
	})

	served := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer close(served)
		r.ServeHTTP(w, req)
	}))
	defer srv.Close()

	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte("GET /ws HTTP/1.1\r\nHost: example.test\r\nConnection: Upgrade\r\nUpgrade: test\r\n\r\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusSwitchingProtocols)
	}

	// The handler is still holding the connection open.
	summaries := logs.FilterMessage("Request summary statistics").AllUntimed()
	if len(summaries) != 1 {
		t.Fatalf("summaries before close = %d, want 1: %#v", len(summaries), logs.AllUntimed())
	}
	summary := summaries[0]
	if summary.Level != zapcore.DebugLevel {
		t.Errorf("summary level = %v, want %v", summary.Level, zapcore.DebugLevel)
	}
	assertObservedFieldOnce(t, summary, logkeys.Path, "/ws")
	assertObservedFieldOnce(t, summary, logkeys.Hijacked, true)

	close(release)
	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return after release")
	}
	if got := logs.FilterMessage("Request summary statistics").Len(); got != 1 {
		t.Errorf("summaries after close = %d, want 1: %#v", got, logs.AllUntimed())
	}
}

// TestRequestSummaryDurationIsNumericMilliseconds keeps the summary duration
// sortable regardless of the application's zap duration encoder.
func TestRequestSummaryDurationIsNumericMilliseconds(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	r := NewRouter(RouterConfig{
		Logger:             zap.New(core),
		EnableTraceLogging: true,
	}, RouterDependencies[string, struct{}]{})
	r.Route(RouteConfigBase{
		Path:    "/slow",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(5 * time.Millisecond)
			w.WriteHeader(http.StatusNoContent)
		},
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/slow", nil))

	summaries := logs.FilterMessage("Request summary statistics").AllUntimed()
	if len(summaries) != 1 {
		t.Fatalf("summaries = %d, want 1: %#v", len(summaries), logs.AllUntimed())
	}
	fields := summaries[0].ContextMap()
	ms, ok := fields[logkeys.DurationMS].(float64)
	if !ok {
		t.Fatalf("%s = %#v, want float64", logkeys.DurationMS, fields[logkeys.DurationMS])
	}
	if ms < 5 || ms > 5000 {
		t.Errorf("%s = %v, want at least 5 and well under 5000", logkeys.DurationMS, ms)
	}
	if _, present := fields[logkeys.Hijacked]; present {
		t.Errorf("%s emitted for a non-hijacked request: %#v", logkeys.Hijacked, fields)
	}
}

// TestFailedHijackKeepsSummaryAtCompletion verifies that an unsupported hijack
// leaves the normal end-of-request summary in place.
func TestFailedHijackKeepsSummaryAtCompletion(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	r := NewRouter(RouterConfig{
		Logger:             zap.New(core),
		EnableTraceLogging: true,
	}, RouterDependencies[string, struct{}]{})
	r.Route(RouteConfigBase{
		Path:    "/ws",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			if _, _, err := http.NewResponseController(w).Hijack(); err == nil {
				t.Error("Hijack() on a ResponseRecorder succeeded, want error")
			}
			w.WriteHeader(http.StatusBadRequest)
		},
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ws", nil))

	summaries := logs.FilterMessage("Request summary statistics").AllUntimed()
	if len(summaries) != 1 {
		t.Fatalf("summaries = %d, want 1: %#v", len(summaries), logs.AllUntimed())
	}
	assertObservedFieldOnce(t, summaries[0], logkeys.Status, int64(http.StatusBadRequest))
	if _, present := summaries[0].ContextMap()[logkeys.Hijacked]; present {
		t.Errorf("%s emitted after a failed hijack", logkeys.Hijacked)
	}
}
