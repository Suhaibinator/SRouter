// Run with go run . and call:
// curl -H 'Authorization: Bearer example-token' -H 'X-Trace-ID: demo' localhost:8083/admin
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/router"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
)

type userID uint64

type user struct {
	id userID
}

// The service owns its relative name and its fallback for non-router contexts.
const adminName = "common_service.admin"

type adminHandler struct {
	fallback *zap.Logger
}

func newAdminHandler(appLogger *zap.Logger) *adminHandler {
	return &adminHandler{fallback: appLogger.Named(adminName)}
}

func (h *adminHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logger, ok := scontext.GetLogger[userID, user](req.Context())
	if ok {
		logger = logger.Named(adminName)
	} else {
		logger = h.fallback
	}
	logger.Info("admin operation started")
	// Both lines share the encoded correlation and the service name.
	logger.Info("admin operation completed")
	w.WriteHeader(http.StatusNoContent)
}

func main() {
	base, err := zap.NewProduction()
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = base.Sync() }()
	appLogger := base.Named("example")

	r := router.NewRouter(router.RouterConfig{
		Logger:            appLogger,
		TraceIDBufferSize: 100,
	}, router.RouterDependencies[userID, user]{
		Authenticate: func(_ context.Context, token string) (*user, bool) {
			if token != "example-token" {
				return nil, false
			}
			return &user{id: 4242}, true
		},
		UserID:   func(u *user) userID { return u.id },
		BuildID:  func() string { return "example-build" },
		ConfigID: func() string { return "example-config" },
		// The default startup encoder handles the named uint64 userID type.
	})
	defer func() { _ = r.Shutdown(context.Background()) }()
	admin := newAdminHandler(appLogger)
	auth := router.AuthRequired
	r.Route(router.RouteConfigBase{
		Path:      "/admin",
		Methods:   []router.HttpMethod{router.MethodGet},
		AuthLevel: &auth,
		Handler:   admin.ServeHTTP,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Addr: ":8083", Handler: r, ReadHeaderTimeout: 5 * time.Second}
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			appLogger.Error("server shutdown failed", zap.Error(err))
		}
	}()
	appLogger.Info("listening", zap.String("address", server.Addr))
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		appLogger.Error("server failed", zap.Error(err))
	}
	stop()
	<-done
}
