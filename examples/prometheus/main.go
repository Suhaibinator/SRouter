package main

import (
	"fmt"
	"log"
	"net/http"

	srouterprom "github.com/Suhaibinator/SRouter/pkg/metrics/prometheus"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

func newApplication() (http.Handler, error) {
	nativeRegistry := prometheus.NewRegistry()
	registry := srouterprom.NewPrometheusRegistry(
		nativeRegistry,
		"example",
		"api",
		zap.NewNop(),
	)

	r := router.NewRouter[string, struct{}](router.RouterConfig{
		ServiceName: "prometheus-example",
		Logger:      zap.NewNop(),
		MetricsConfig: &router.MetricsConfig{
			Collector:     registry,
			EnableLatency: true,
			EnableQPS:     true,
			EnableErrors:  true,
		},
	}, router.RouterDependencies[string, struct{}]{})

	r.Route(
		router.RouteConfigBase{
			Path:    "/api/hello",
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintln(w, `{"message":"hello"}`)
			},
		},
		router.RouteConfigBase{
			Path:    "/api/error",
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "example failure", http.StatusInternalServerError)
			},
		},
	)

	if err := r.Build(); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(nativeRegistry, promhttp.HandlerOpts{}))
	mux.Handle("/", r)
	return mux, nil
}

func main() {
	handler, err := newApplication()
	if err != nil {
		log.Fatal(err)
	}

	log.Print("listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}
