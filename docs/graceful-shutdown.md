# Graceful Shutdown

Properly shutting down your HTTP server is crucial to avoid abruptly terminating active connections and potentially losing data or leaving operations in an inconsistent state. SRouter provides mechanisms to support graceful shutdown.

## Router Shutdown Method

The SRouter instance (`*router.Router`) itself has a `Shutdown` method. This method is primarily responsible for signaling internal components, like the rate limiter (if used), to stop accepting new requests and potentially clean up resources.

```go
// func (r *Router[T, U]) Shutdown(ctx context.Context) error
```

It takes a `context.Context` which can be used to set a deadline for the shutdown process.

## Integrating with `http.Server` Shutdown

The standard Go `net/http` package provides the `http.Server.Shutdown` method for gracefully shutting down the server. This method stops the server from accepting new connections and waits for active requests to complete before returning.

You should call both the `http.Server.Shutdown` and the `router.Router.Shutdown` methods when handling termination signals (like `SIGINT` or `SIGTERM`).

```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall" // Use syscall for SIGTERM if needed
	"time"

	"github.com/Suhaibinator/SRouter/pkg/router" // Assuming router setup as 'r'
	"go.uber.org/zap"                           // Assuming logger setup as 'logger'
)

func main() {
	// --- Assume router 'r' is configured and created here ---
	logger, _ := zap.NewProduction() // Example logger
	defer logger.Sync()
	// ... router config ...
	r := router.NewRouter[string, string](/* ... config, auth funcs ... */)
	// ... register routes ...

	// Create the HTTP server
	srv := &http.Server{
		Addr:    ":8080",
		Handler: r, // Use the SRouter instance as the handler
		// Add other server configurations like ReadTimeout, WriteTimeout if desired
	}

	// Start the server in a separate goroutine so it doesn't block
	go func() {
		log.Println("Server starting on", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe failed: %v", err)
		}
	}()

	// Wait for interrupt signal (SIGINT) or termination signal (SIGTERM)
	quit := make(chan os.Signal, 1)
	// signal.Notify(quit, os.Interrupt) // Catch only Ctrl+C
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM) // Catch Ctrl+C and SIGTERM
	receivedSignal := <-quit
	log.Printf("Received signal: %s. Shutting down server...\n", receivedSignal)

	// Create a context with a timeout for the shutdown process
	// Give active requests time to finish (e.g., 5 seconds)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// --- Perform Shutdown ---

	// 1. Shut down the SRouter instance (signals internal components like rate limiter)
	// It's generally good practice to shut down the router logic first.
	if err := r.Shutdown(ctx); err != nil {
		log.Printf("SRouter shutdown failed: %v\n", err)
		// Decide if this error is fatal or if you should proceed
	} else {
		log.Println("SRouter shut down successfully.")
	}

	// 2. Shut down the HTTP server (stops accepting new connections, waits for active requests)
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("HTTP server shutdown failed: %v", err) // Often fatal if server can't shut down
	} else {
		log.Println("HTTP server shut down successfully.")
	}

	log.Println("Server exited gracefully")
}

```

### Shutdown Order

1.  **Receive Signal**: Detect `SIGINT` or `SIGTERM`.
2.  **Create Context**: Create a `context.WithTimeout` to limit the shutdown duration.
3.  **Router Shutdown**: Call `r.Shutdown(ctx)` to signal SRouter's internal components.
4.  **Server Shutdown**: Call `srv.Shutdown(ctx)` to stop accepting new connections and wait for existing requests handled by SRouter to complete.

This ensures that the server stops accepting new work, SRouter components are notified, and active requests are given time to finish before the application exits.

See the `examples/graceful-shutdown` directory for a runnable example.
