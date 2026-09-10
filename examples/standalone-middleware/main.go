// Run with go run . to emit a handler log and a correlated rate-limit warning
// without starting a server. No Router is needed: install the shared source at
// the HTTP boundary, before middleware that may log.
package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/middleware"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
)

type user struct{}

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = logger.Sync() }()
	source := scontext.NewRequestLoggerSource[string](logger.Named("example"), nil)
	attachLogger := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := scontext.WithRequestLogger[string, user](req.Context(), source)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}
	generator := middleware.NewIDGenerator(1)
	defer generator.Stop()
	handler := middleware.Chain(
		attachLogger,
		router.ClientIPMiddleware[string, user](&router.IPConfig{Source: router.IPSourceRemoteAddr}),
		middleware.CreateTraceMiddleware[string, user](generator),
		middleware.Recovery[string, user](),
		middleware.RateLimit(&common.RateLimitConfig[string, user]{
			BucketName: "standalone", Strategy: common.StrategyIP, Limit: 1, Window: time.Minute,
		}, middleware.NewUberRateLimiter()),
	)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if logger, ok := scontext.GetLogger[string, user](req.Context()); ok {
			logger.Named("handler").Info("Request accepted")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	for i := range 2 {
		req := httptest.NewRequest(http.MethodGet, "/limited", nil)
		req.RemoteAddr = "192.0.2.44:9000"
		req.Header.Set("X-Trace-ID", fmt.Sprintf("demo-%d", i+1))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		fmt.Printf("request %d: status=%d trace=%s\n", i+1, response.Code, response.Header().Get("X-Trace-ID"))
	}
}
