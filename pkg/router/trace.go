package router

import (
	"net/http"
	"unicode"
	"unicode/utf8"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"github.com/Suhaibinator/SRouter/pkg/traceid"
)

// resolveTraceID runs once after context/logger initialization, including for
// requests that never reach routing. Generated fallbacks bypass user validation.
func (r *Router[T, U]) resolveTraceID(w http.ResponseWriter, req *http.Request) {
	config := r.config.TraceIDConfig
	if config == nil {
		return
	}
	valid := func(id string) bool { return safeTraceID(id) && config.Validator(id) }
	id := scontext.GetTraceID[T, U](req.Context())
	if !valid(id) {
		var ok bool
		id, ok = config.Source(req)
		if !ok || !valid(id) {
			if r.traceIDGenerator != nil {
				id = r.traceIDGenerator.Next()
			} else {
				// Invalid buffer sizes must still correlate lazy-build failures.
				id = traceid.Generate()
			}
		}
	}
	// withRequestLogging has already installed the shared mutable context.
	scontext.SetTraceID[T, U](req.Context(), id)
	w.Header().Set(config.ResponseHeader, id)
}

func safeTraceID(id string) bool {
	if id == "" || len(id) > 64 || !utf8.ValidString(id) {
		return false
	}
	for _, c := range id {
		if unicode.IsSpace(c) || unicode.IsControl(c) {
			return false
		}
	}
	return true
}
