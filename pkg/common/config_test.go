package common

import "testing"

func TestRouteOverridesHasRateLimit(t *testing.T) {
	overrides := RouteOverrides{}
	if overrides.HasRateLimit() {
		t.Fatal("expected HasRateLimit to be false when rate limit is nil")
	}

	overrides.RateLimit = &RateLimitConfig[any, any]{}
	if !overrides.HasRateLimit() {
		t.Fatal("expected HasRateLimit to be true when rate limit is set")
	}
}

func TestRouteOverridesHasAuthToken(t *testing.T) {
	overrides := RouteOverrides{}
	if overrides.HasAuthToken() {
		t.Fatal("expected HasAuthToken to be false when auth token is nil")
	}

	overrides.AuthToken = &AuthTokenConfig{}
	if !overrides.HasAuthToken() {
		t.Fatal("expected HasAuthToken to be true when auth token is set")
	}
}
