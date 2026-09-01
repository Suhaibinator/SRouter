package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Suhaibinator/SRouter/pkg/router"
	"go.uber.org/zap"
)

var (
	omens = []string{
		"The route is open",
		"A middleware spirit says yes",
		"The types align in your favor",
		"Retry after one heroic refactor",
		"All status codes point to yes",
	}
	runes = []string{"ᚠ", "ᚢ", "ᚦ", "ᚨ", "ᚱ"}
)

// consultOracle receives an ordinary typed value. It does not know or care
// whether that value arrived as JSON, protobuf, or a rune scroll.
func consultOracle(_ *http.Request, request OracleRequest) (OracleResponse, error) {
	digest := sha256.Sum256([]byte(strings.ToLower(request.Question + "|" + request.Vibe)))

	return OracleResponse{
		Seeker:    request.Seeker,
		Question:  request.Question,
		Vibe:      request.Vibe,
		Omen:      omens[int(digest[0])%len(omens)],
		LuckyRune: runes[int(digest[1])%len(runes)],
	}, nil
}

// sanitizeOracleRequest keeps syntax concerns in the codec and business input
// concerns here, where they can be reused no matter which request source is used.
func sanitizeOracleRequest(_ context.Context, request OracleRequest) (OracleRequest, error) {
	request.Seeker = oneLine(request.Seeker)
	request.Question = oneLine(request.Question)
	request.Vibe = strings.ToLower(oneLine(request.Vibe))

	if request.Seeker == "" {
		return request, router.NewHTTPError(http.StatusBadRequest, "Every scroll needs a seeker")
	}
	if request.Question == "" {
		return request, router.NewHTTPError(http.StatusBadRequest, "The oracle needs a question")
	}
	if utf8.RuneCountInString(request.Question) > 160 {
		return request, router.NewHTTPError(http.StatusBadRequest, "Questions must be 160 characters or fewer")
	}

	switch request.Vibe {
	case "curious", "electric", "mysterious":
		return request, nil
	default:
		return request, router.NewHTTPError(
			http.StatusBadRequest,
			"Vibe must be curious, electric, or mysterious",
		)
	}
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func newOracleRouter() *router.Router[string, string] {
	authenticate := func(_ context.Context, _ string) (*string, bool) {
		return nil, false
	}
	userID := func(user *string) string {
		if user == nil {
			return ""
		}
		return *user
	}

	r := router.NewRouter(router.RouterConfig{
		ServiceName:       "rune-scroll-oracle",
		Logger:            zap.NewNop(),
		GlobalTimeout:     2 * time.Second,
		GlobalMaxBodySize: 8 << 10,
	}, router.RouterDependencies[string, string]{Authenticate: authenticate, UserID: userID})

	runeCodec := NewRuneScrollCodec()
	for _, route := range []router.RouteConfig[OracleRequest, OracleResponse]{
		{
			Path:       "/oracle",
			Methods:    []router.HttpMethod{router.MethodPost},
			Codec:      runeCodec,
			Handler:    consultOracle,
			Sanitizer:  sanitizeOracleRequest,
			SourceType: router.Body,
		},
		{
			Path:       "/oracle",
			Methods:    []router.HttpMethod{router.MethodGet},
			Codec:      runeCodec,
			Handler:    consultOracle,
			Sanitizer:  sanitizeOracleRequest,
			SourceType: router.Base64QueryParameter,
			SourceKey:  "scroll",
		},
	} {
		r.Route(route)
	}

	return r
}

func main() {
	r := newOracleRouter()

	fmt.Println("Rune Scroll custom codec listening on http://localhost:8080")
	fmt.Println("Try the POST and GET examples in examples/custom-codec/README.md")

	server := &http.Server{
		Addr:              ":8080",
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}
