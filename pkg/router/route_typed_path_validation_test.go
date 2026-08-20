package router

import (
	"net/http"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBuildRejectsTypedRouteWithInvalidPath(t *testing.T) {
	r := NewRouter(RouterConfig{Logger: zap.NewNop()}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	r.Route(RouteConfig[struct{}, struct{}]{
		Path:    "relative",
		Methods: []HttpMethod{MethodGet},
		Codec:   codec.NewJSONCodec[struct{}, struct{}](),
		Handler: func(*http.Request, struct{}) (struct{}, error) {
			t.Fatal("handler should not be called for an invalid route")
			return struct{}{}, nil
		},
	})

	err := r.Build()
	require.EqualError(t, err, `route path "relative" must begin with '/'`)
}
