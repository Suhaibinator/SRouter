package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRouterRootConfigurationMethodsAreFluent(t *testing.T) {
	r := NewRouter(RouterConfig{Logger: zap.NewNop()}, RouterDependencies[string, string]{Authenticate: mocks.MockAuthFunction, UserID: mocks.MockUserIDFromUser})
	middlewareCalled := false

	require.Same(t, r, r.Timeout(0))
	require.Same(t, r, r.MaxBodySize(0))
	require.Same(t, r, r.RateLimit(nil))
	require.Same(t, r, r.AuthToken(nil))
	require.Same(t, r, r.Auth(NoAuth))
	require.Same(t, r, r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			middlewareCalled = true
			next.ServeHTTP(w, req)
		})
	}))
	r.Route(RouteConfigBase{
		Path:    "/test",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	})

	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))

	require.True(t, middlewareCalled)
	require.Equal(t, http.StatusNoContent, recorder.Code)
}
