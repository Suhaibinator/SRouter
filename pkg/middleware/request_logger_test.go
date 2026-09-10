package middleware

import (
	"net/http"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
)

// rateLimitWithLogger preserves the focused rate-limit log assertions while
// exercising the public middleware API with a router-like request logger.
func rateLimitWithLogger[T comparable, U any](
	config *common.RateLimitConfig[T, U],
	limiter common.RateLimiter,
	logger *zap.Logger,
) common.Middleware {
	rateLimit := RateLimit(config, limiter)
	if logger == nil {
		return rateLimit
	}
	source := scontext.NewRequestLoggerSource[T](logger, nil)
	return func(next http.Handler) http.Handler {
		handler := rateLimit(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := scontext.WithRequestLogger[T, U](r.Context(), source)
			handler.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
