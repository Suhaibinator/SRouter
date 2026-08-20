package router

import (
	"strings"
	"testing"
)

func TestRouteGroupMutateRejectsNilGroups(t *testing.T) {
	tests := []struct {
		name  string
		group *RouteGroup[string, string]
	}{
		{name: "nil receiver"},
		{name: "missing route tree", group: &RouteGroup[string, string]{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				panicValue := recover()
				if panicValue != "router: nil route group" {
					t.Fatalf("expected nil route group panic, got %v", panicValue)
				}
			}()

			tt.group.mutate(func() {
				t.Fatal("mutation callback must not run")
			})
		})
	}
}

func TestValidateGroupPrefixErrors(t *testing.T) {
	tests := []struct {
		name        string
		prefix      string
		wantErrText string
	}{
		{name: "empty", prefix: "", wantErrText: "must not be empty"},
		{name: "trailing slash", prefix: "/api/", wantErrText: "must not end with '/'"},
		{name: "query", prefix: "/api?version=1", wantErrText: "must not contain '?' or '#'"},
		{name: "fragment", prefix: "/api#users", wantErrText: "must not contain '?' or '#'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGroupPrefix(tt.prefix)
			if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
				t.Fatalf("validateGroupPrefix(%q) error = %v, want error containing %q", tt.prefix, err, tt.wantErrText)
			}
		})
	}
}

func TestJoinGroupPathRootChild(t *testing.T) {
	if got := joinGroupPath("/api", "/"); got != "/api" {
		t.Fatalf("joinGroupPath root child = %q, want %q", got, "/api")
	}
}

func TestJoinRoutePathErrors(t *testing.T) {
	tests := []struct {
		name        string
		prefix      string
		path        string
		wantErrText string
	}{
		{name: "empty root path", wantErrText: "root route path must not be empty"},
		{name: "missing leading slash", prefix: "/api", path: "users", wantErrText: "must begin with '/'"},
		{name: "query", prefix: "/api", path: "/users?active=true", wantErrText: "must not contain '?' or '#'"},
		{name: "fragment", prefix: "/api", path: "/users#active", wantErrText: "must not contain '?' or '#'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := joinRoutePath(tt.prefix, tt.path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
				t.Fatalf("joinRoutePath(%q, %q) = %q, %v; want error containing %q", tt.prefix, tt.path, got, err, tt.wantErrText)
			}
			if got != "" {
				t.Fatalf("joinRoutePath(%q, %q) path = %q, want empty path on error", tt.prefix, tt.path, got)
			}
		})
	}
}
