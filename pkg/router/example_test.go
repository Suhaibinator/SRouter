package router_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/Suhaibinator/SRouter/pkg/router"
)

func ExampleNewRouter() {
	r := router.NewRouter[string, string](router.RouterConfig{
		ServiceName: "hello-service",
	}, nil, nil)

	r.Route(router.RouteConfigBase{
		Path:    "/hello",
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":"hello"}`))
		},
	})
	if err := r.Build(); err != nil {
		panic(err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/hello", nil)
	r.ServeHTTP(response, request)

	fmt.Println(response.Code)
	fmt.Println(response.Body.String())
	// Output:
	// 200
	// {"message":"hello"}
}
