package router

import (
	"net/http"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
)

// createTransactionMiddleware creates a middleware that wraps the handler with transaction management.
// It begins a transaction, adds it to the context, and commits/rollbacks based on the response status.
// This is an internal helper to reduce code duplication across different route registration methods.
func createTransactionMiddleware[T comparable, U any](
	r *Router[T, U],
	transaction *common.TransactionConfig,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// Begin transaction
			tx, err := r.config.TransactionFactory.BeginTransaction(req.Context(), transaction.Options)
			if err != nil {
				r.handleError(w, req, err, http.StatusInternalServerError, "Failed to begin transaction")
				return
			}

			// Add transaction to context
			ctx := scontext.WithTransaction[T, U](req.Context(), tx)
			req = req.WithContext(ctx)

			// Create status-capturing writer
			captureWriter := &statusCapturingResponseWriter{ResponseWriter: w}

			// Call the next handler
			next.ServeHTTP(captureWriter, req)

			// Determine if handler succeeded based on status code
			// Consider 2xx and 3xx as success
			success := captureWriter.status >= 200 && captureWriter.status < 400

			// Commit or rollback based on success
			if success {
				if err := tx.Commit(); err != nil {
					fields := append(r.baseFields(req), zap.Error(err))
					fields = r.addTrace(fields, req)
					r.logger.Error("Failed to commit transaction", fields...)
					// Note: We can't change the response at this point
				}
			} else {
				if err := tx.Rollback(); err != nil {
					fields := append(r.baseFields(req), zap.Error(err))
					fields = r.addTrace(fields, req)
					r.logger.Error("Failed to rollback transaction", fields...)
				}
			}
		})
	}
}

// wrapWithTransaction wraps a handler with transaction middleware if enabled.
// Returns the original handler if transactions are not enabled or configured.
func (r *Router[T, U]) wrapWithTransaction(
	handler http.Handler,
	transaction *common.TransactionConfig,
) http.Handler {
	if transaction != nil && transaction.Enabled && r.config.TransactionFactory != nil {
		middleware := createTransactionMiddleware(r, transaction)
		return middleware(handler)
	}
	return handler
}