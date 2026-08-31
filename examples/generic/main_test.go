package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/router"
	"go.uber.org/zap"
)

func TestBodylessRoutesReachTheirHandlers(t *testing.T) {
	r := router.NewRouter[string, string](
		router.RouterConfig{Logger: zap.NewNop()},
		nil,
		nil,
	)
	registerRoutes(r)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{name: "get user", method: http.MethodGet, path: "/users/1", wantStatus: http.StatusOK},
		{name: "list users", method: http.MethodGet, path: "/users", wantStatus: http.StatusOK},
		{name: "delete missing user", method: http.MethodDelete, path: "/users/not-present", wantStatus: http.StatusNotFound},
		{name: "deliberate handler error", method: http.MethodGet, path: "/error", wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, tt.path, nil)
			r.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
		})
	}
}

func TestUserStoreConcurrentAccess(t *testing.T) {
	store := newUserStore(map[string]User{
		"1": {ID: "1", Name: "Existing", Email: "existing@example.com"},
	}, 2)

	const workers = 32
	var wg sync.WaitGroup
	ids := make(chan string, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			user := store.create(fmt.Sprintf("User %d", i), fmt.Sprintf("user%d@example.com", i))
			ids <- user.ID
			if _, ok := store.get(user.ID); !ok {
				t.Errorf("created user %q was not found", user.ID)
			}
			if _, ok := store.update(user.ID, "Updated", user.Email); !ok {
				t.Errorf("created user %q could not be updated", user.ID)
			}
			_ = store.list()
		}(i)
	}

	wg.Wait()
	close(ids)

	seen := make(map[string]struct{}, workers)
	for id := range ids {
		if _, duplicate := seen[id]; duplicate {
			t.Fatalf("duplicate ID allocated: %q", id)
		}
		seen[id] = struct{}{}
	}

	if got := len(store.list()); got != workers+1 {
		t.Fatalf("store contains %d users; want %d", got, workers+1)
	}
}

func TestUserStoreDoesNotReuseDeletedID(t *testing.T) {
	store := newUserStore(nil, 1)
	first := store.create("First", "first@example.com")
	if !store.delete(first.ID) {
		t.Fatalf("failed to delete user %q", first.ID)
	}

	second := store.create("Second", "second@example.com")
	if second.ID == first.ID {
		t.Fatalf("reused deleted ID %q", second.ID)
	}
}
