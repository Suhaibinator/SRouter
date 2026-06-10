package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
)

// End-to-end tests in this file exercise the full stack over a real TCP
// connection: a router served by httptest.NewServer and driven by a real
// http.Client, rather than calling ServeHTTP with a recorder directly.

type e2eUser struct {
	ID   string
	Name string
}

type e2eCreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type e2eCreateUserResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// newE2EAuthFunctions returns auth functions that accept the tokens in the
// given map (token -> user name) and reject everything else.
func newE2EAuthFunctions(tokens map[string]string) (func(context.Context, string) (*e2eUser, bool), func(*e2eUser) string) {
	authFunc := func(ctx context.Context, token string) (*e2eUser, bool) {
		name, ok := tokens[token]
		if !ok {
			return nil, false
		}
		return &e2eUser{ID: "id-" + name, Name: name}, true
	}
	userIDFunc := func(u *e2eUser) string {
		if u == nil {
			return ""
		}
		return u.ID
	}
	return authFunc, userIDFunc
}

// TestE2EFullStackAPI runs a complete API server over a real HTTP connection:
// trace IDs, global and sub-router middleware, a declarative generic JSON
// route, bearer-token authentication, and standard routing behavior
// (404/405) all working together.
func TestE2EFullStackAPI(t *testing.T) {
	authFunc, userIDFunc := newE2EAuthFunctions(map[string]string{"token-alice": "alice"})

	r := NewRouter(RouterConfig{
		Logger:             zap.NewNop(),
		GlobalTimeout:      2 * time.Second,
		GlobalMaxBodySize:  1 << 20,
		TraceIDBufferSize:  10,
		AddUserObjectToCtx: true,
		Middlewares: []common.Middleware{
			func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Set("X-Global-Middleware", "applied")
					next.ServeHTTP(w, req)
				})
			},
		},
		SubRouters: []SubRouterConfig{
			{
				PathPrefix: "/api/v1",
				Middlewares: []common.Middleware{
					func(next http.Handler) http.Handler {
						return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							w.Header().Set("X-API-Version", "v1")
							next.ServeHTTP(w, req)
						})
					},
				},
				Routes: []RouteDefinition{
					RouteConfigBase{
						Path:    "/health",
						Methods: []HttpMethod{MethodGet},
						Handler: func(w http.ResponseWriter, req *http.Request) {
							w.Header().Set("Content-Type", "application/json")
							_, _ = w.Write([]byte(`{"status":"ok"}`))
						},
					},
					RouteConfigBase{
						Path:      "/me",
						Methods:   []HttpMethod{MethodGet},
						AuthLevel: new(AuthRequired),
						Handler: func(w http.ResponseWriter, req *http.Request) {
							userID, ok := scontext.GetUserIDFromRequest[string, e2eUser](req)
							if !ok {
								http.Error(w, "no user in context", http.StatusInternalServerError)
								return
							}
							user, ok := scontext.GetUserFromRequest[string, e2eUser](req)
							if !ok || user == nil {
								http.Error(w, "no user object in context", http.StatusInternalServerError)
								return
							}
							w.Header().Set("Content-Type", "application/json")
							_, _ = fmt.Fprintf(w, `{"id":%q,"name":%q}`, userID, user.Name)
						},
					},
					NewGenericRouteDefinition[e2eCreateUserRequest, e2eCreateUserResponse, string, e2eUser](
						RouteConfig[e2eCreateUserRequest, e2eCreateUserResponse]{
							Path:    "/users",
							Methods: []HttpMethod{MethodPost},
							Codec:   codec.NewJSONCodec[e2eCreateUserRequest, e2eCreateUserResponse](),
							Sanitizer: func(req e2eCreateUserRequest) (e2eCreateUserRequest, error) {
								req.Name = strings.TrimSpace(req.Name)
								return req, nil
							},
							Handler: func(req *http.Request, data e2eCreateUserRequest) (e2eCreateUserResponse, error) {
								return e2eCreateUserResponse{
									ID:    "user-1",
									Name:  data.Name,
									Email: data.Email,
								}, nil
							},
						},
					),
				},
			},
		},
	}, authFunc, userIDFunc)

	server := httptest.NewServer(r)
	defer server.Close()
	client := server.Client()

	t.Run("public route with middleware and trace ID", func(t *testing.T) {
		resp, err := client.Get(server.URL + "/api/v1/health")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != `{"status":"ok"}` {
			t.Errorf("unexpected body: %s", body)
		}
		if resp.Header.Get("X-Global-Middleware") != "applied" {
			t.Errorf("expected global middleware header, got %q", resp.Header.Get("X-Global-Middleware"))
		}
		if resp.Header.Get("X-API-Version") != "v1" {
			t.Errorf("expected sub-router middleware header, got %q", resp.Header.Get("X-API-Version"))
		}
		if resp.Header.Get("X-Trace-ID") == "" {
			t.Error("expected X-Trace-ID response header to be set")
		}
	})

	t.Run("generic JSON route round-trip", func(t *testing.T) {
		reqBody := `{"name":"  Bob  ","email":"bob@example.com"}`
		resp, err := client.Post(server.URL+"/api/v1/users", "application/json", strings.NewReader(reqBody))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}
		var created e2eCreateUserResponse
		if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if created.ID != "user-1" {
			t.Errorf("expected ID %q, got %q", "user-1", created.ID)
		}
		if created.Name != "Bob" {
			t.Errorf("expected sanitized name %q, got %q", "Bob", created.Name)
		}
		if created.Email != "bob@example.com" {
			t.Errorf("expected email %q, got %q", "bob@example.com", created.Email)
		}
	})

	t.Run("generic route rejects malformed JSON", func(t *testing.T) {
		resp, err := client.Post(server.URL+"/api/v1/users", "application/json", strings.NewReader("{not json"))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
		}
	})

	t.Run("auth required without token", func(t *testing.T) {
		resp, err := client.Get(server.URL + "/api/v1/me")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, resp.StatusCode)
		}
	})

	t.Run("auth required with invalid token", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/me", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, resp.StatusCode)
		}
	})

	t.Run("auth required with valid token", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/me", nil)
		req.Header.Set("Authorization", "Bearer token-alice")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		expected := `{"id":"id-alice","name":"alice"}`
		if string(body) != expected {
			t.Errorf("expected body %q, got %q", expected, body)
		}
	})

	t.Run("unknown path returns 404", func(t *testing.T) {
		resp, err := client.Get(server.URL + "/api/v1/nope")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, resp.StatusCode)
		}
	})

	t.Run("wrong method returns 405", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/v1/health", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, resp.StatusCode)
		}
	})
}

// TestE2ERateLimiting verifies that IP-based rate limiting is enforced for
// real HTTP clients, including the X-RateLimit-* and Retry-After headers.
func TestE2ERateLimiting(t *testing.T) {
	authFunc, userIDFunc := newE2EAuthFunctions(nil)

	const limit = 3
	r := NewRouter(RouterConfig{
		Logger: zap.NewNop(),
	}, authFunc, userIDFunc)

	r.RegisterRoute(RouteConfigBase{
		Path:    "/limited",
		Methods: []HttpMethod{MethodGet},
		Overrides: common.RouteOverrides{
			RateLimit: &common.RateLimitConfig[any, any]{
				BucketName: "e2e-limited",
				Limit:      limit,
				Window:     time.Minute,
				Strategy:   common.StrategyIP,
			},
		},
		Handler: func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	})

	server := httptest.NewServer(r)
	defer server.Close()
	client := server.Client()

	for i := 1; i <= limit; i++ {
		resp, err := client.Get(server.URL + "/limited")
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: expected status %d, got %d", i, http.StatusOK, resp.StatusCode)
		}
		if got := resp.Header.Get("X-RateLimit-Limit"); got != fmt.Sprint(limit) {
			t.Errorf("request %d: expected X-RateLimit-Limit %d, got %q", i, limit, got)
		}
		if got := resp.Header.Get("X-RateLimit-Remaining"); got != fmt.Sprint(limit-i) {
			t.Errorf("request %d: expected X-RateLimit-Remaining %d, got %q", i, limit-i, got)
		}
	}

	// The next request from the same IP must be rejected.
	resp, err := client.Get(server.URL + "/limited")
	if err != nil {
		t.Fatalf("over-limit request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected status %d, got %d", http.StatusTooManyRequests, resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}
}

// TestE2ECORS verifies CORS preflight and actual cross-origin requests over a
// real HTTP connection.
func TestE2ECORS(t *testing.T) {
	authFunc, userIDFunc := newE2EAuthFunctions(nil)

	r := NewRouter(RouterConfig{
		Logger: zap.NewNop(),
		CORSConfig: &CORSConfig{
			Origins:          []string{"http://example.com"},
			Methods:          []string{"GET", "POST"},
			Headers:          []string{"Content-Type", "Authorization"},
			AllowCredentials: true,
			MaxAge:           time.Hour,
		},
	}, authFunc, userIDFunc)

	r.RegisterRoute(RouteConfigBase{
		Path:    "/resource",
		Methods: []HttpMethod{MethodGet, MethodPost},
		Handler: func(w http.ResponseWriter, req *http.Request) {
			_, _ = w.Write([]byte("resource"))
		},
	})

	server := httptest.NewServer(r)
	defer server.Close()
	client := server.Client()

	t.Run("preflight request", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodOptions, server.URL+"/resource", nil)
		req.Header.Set("Origin", "http://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "Content-Type")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("preflight request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode >= 300 {
			t.Fatalf("expected preflight success, got status %d", resp.StatusCode)
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://example.com" {
			t.Errorf("expected Access-Control-Allow-Origin %q, got %q", "http://example.com", got)
		}
		if got := resp.Header.Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
			t.Errorf("expected Access-Control-Allow-Methods to contain POST, got %q", got)
		}
		if got := resp.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Errorf("expected Access-Control-Allow-Credentials true, got %q", got)
		}
	})

	t.Run("actual cross-origin request", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/resource", nil)
		req.Header.Set("Origin", "http://example.com")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "resource" {
			t.Errorf("expected body %q, got %q", "resource", body)
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://example.com" {
			t.Errorf("expected Access-Control-Allow-Origin %q, got %q", "http://example.com", got)
		}
	})

	t.Run("disallowed origin gets no CORS headers", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/resource", nil)
		req.Header.Set("Origin", "http://evil.example.org")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("expected no Access-Control-Allow-Origin header, got %q", got)
		}
	})
}

// TestE2ETimeoutAndPanicRecovery verifies that timeouts and panic recovery
// produce proper HTTP error responses over a real connection, and that the
// server keeps serving normally afterwards.
func TestE2ETimeoutAndPanicRecovery(t *testing.T) {
	authFunc, userIDFunc := newE2EAuthFunctions(nil)

	r := NewRouter(RouterConfig{
		Logger:        zap.NewNop(),
		GlobalTimeout: 100 * time.Millisecond,
	}, authFunc, userIDFunc)

	r.RegisterRoute(RouteConfigBase{
		Path:    "/slow",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, req *http.Request) {
			select {
			case <-time.After(2 * time.Second):
				w.WriteHeader(http.StatusOK)
			case <-req.Context().Done():
			}
		},
	})
	r.RegisterRoute(RouteConfigBase{
		Path:    "/panic",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, req *http.Request) {
			panic("e2e test panic")
		},
	})
	r.RegisterRoute(RouteConfigBase{
		Path:    "/ok",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, req *http.Request) {
			_, _ = w.Write([]byte("still alive"))
		},
	})

	server := httptest.NewServer(r)
	defer server.Close()
	client := server.Client()

	t.Run("slow handler times out", func(t *testing.T) {
		resp, err := client.Get(server.URL + "/slow")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusRequestTimeout {
			t.Errorf("expected status %d, got %d", http.StatusRequestTimeout, resp.StatusCode)
		}
	})

	t.Run("panicking handler returns 500", func(t *testing.T) {
		resp, err := client.Get(server.URL + "/panic")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected status %d, got %d", http.StatusInternalServerError, resp.StatusCode)
		}
	})

	t.Run("server still healthy afterwards", func(t *testing.T) {
		resp, err := client.Get(server.URL + "/ok")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "still alive" {
			t.Errorf("expected body %q, got %q", "still alive", body)
		}
	})
}

// TestE2EConcurrentRequests fires many concurrent real HTTP requests at a
// generic route and verifies every response succeeds with a unique trace ID.
func TestE2EConcurrentRequests(t *testing.T) {
	authFunc, userIDFunc := newE2EAuthFunctions(nil)

	r := NewRouter(RouterConfig{
		Logger:            zap.NewNop(),
		GlobalTimeout:     5 * time.Second,
		TraceIDBufferSize: 100,
	}, authFunc, userIDFunc)

	RegisterGenericRoute(r, RouteConfig[e2eCreateUserRequest, e2eCreateUserResponse]{
		Path:    "/echo",
		Methods: []HttpMethod{MethodPost},
		Codec:   codec.NewJSONCodec[e2eCreateUserRequest, e2eCreateUserResponse](),
		Handler: func(req *http.Request, data e2eCreateUserRequest) (e2eCreateUserResponse, error) {
			return e2eCreateUserResponse{Name: data.Name, Email: data.Email}, nil
		},
	}, time.Duration(0), int64(0), nil)

	server := httptest.NewServer(r)
	defer server.Close()
	client := server.Client()

	const numRequests = 50
	var wg sync.WaitGroup
	var mu sync.Mutex
	traceIDs := make(map[string]bool, numRequests)
	errs := make(chan error, numRequests)

	for i := range numRequests {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			name := fmt.Sprintf("user-%d", i)
			body, _ := json.Marshal(e2eCreateUserRequest{Name: name})
			resp, err := client.Post(server.URL+"/echo", "application/json", bytes.NewReader(body))
			if err != nil {
				errs <- fmt.Errorf("request %d failed: %w", i, err)
				return
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				errs <- fmt.Errorf("request %d: expected status 200, got %d", i, resp.StatusCode)
				return
			}
			var echoed e2eCreateUserResponse
			if err := json.NewDecoder(resp.Body).Decode(&echoed); err != nil {
				errs <- fmt.Errorf("request %d: failed to decode response: %w", i, err)
				return
			}
			if echoed.Name != name {
				errs <- fmt.Errorf("request %d: expected name %q, got %q", i, name, echoed.Name)
				return
			}
			traceID := resp.Header.Get("X-Trace-ID")
			if traceID == "" {
				errs <- fmt.Errorf("request %d: missing X-Trace-ID header", i)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if traceIDs[traceID] {
				errs <- fmt.Errorf("request %d: duplicate trace ID %q", i, traceID)
				return
			}
			traceIDs[traceID] = true
		}(i)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if len(traceIDs) != numRequests {
		t.Errorf("expected %d unique trace IDs, got %d", numRequests, len(traceIDs))
	}
}

// TestE2EGracefulShutdown verifies that an in-flight request completes during
// shutdown and that subsequent requests are rejected with 503.
func TestE2EGracefulShutdown(t *testing.T) {
	authFunc, userIDFunc := newE2EAuthFunctions(nil)

	r := NewRouter(RouterConfig{
		Logger:        zap.NewNop(),
		GlobalTimeout: 2 * time.Second,
	}, authFunc, userIDFunc)

	started := make(chan struct{})
	r.RegisterRoute(RouteConfigBase{
		Path:    "/slow",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, req *http.Request) {
			close(started)
			time.Sleep(100 * time.Millisecond)
			_, _ = w.Write([]byte("completed"))
		},
	})

	server := httptest.NewServer(r)
	defer server.Close()
	client := server.Client()

	type result struct {
		status int
		body   string
		err    error
	}
	inFlight := make(chan result, 1)
	go func() {
		resp, err := client.Get(server.URL + "/slow")
		if err != nil {
			inFlight <- result{err: err}
			return
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		inFlight <- result{status: resp.StatusCode, body: string(body)}
	}()

	// Wait until the handler is actually executing, then shut down.
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight request never reached the handler")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	// The in-flight request must have completed successfully.
	select {
	case res := <-inFlight:
		if res.err != nil {
			t.Fatalf("in-flight request failed: %v", res.err)
		}
		if res.status != http.StatusOK {
			t.Errorf("expected in-flight status %d, got %d", http.StatusOK, res.status)
		}
		if res.body != "completed" {
			t.Errorf("expected in-flight body %q, got %q", "completed", res.body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight request did not complete")
	}

	// New requests after shutdown must be rejected.
	resp, err := client.Get(server.URL + "/slow")
	if err != nil {
		t.Fatalf("post-shutdown request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status %d after shutdown, got %d", http.StatusServiceUnavailable, resp.StatusCode)
	}
}
