package scontext

import (
	"context"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/logkeys"
)

func TestClientIPSettersNormalizeSocketAddresses(t *testing.T) {
	setters := map[string]func(context.Context, string) context.Context{
		"WithClientIP": func(ctx context.Context, ip string) context.Context {
			return WithClientIP[int, testUser](ctx, ip)
		},
		"WithClientInfo": func(ctx context.Context, ip string) context.Context {
			return WithClientInfo[int, testUser](ctx, ip, "test-agent")
		},
	}
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "IPv4 with port", in: "192.0.2.1:1234", want: "192.0.2.1"},
		{name: "IPv4 without port", in: "192.0.2.1", want: "192.0.2.1"},
		{name: "IPv6 with port", in: "[2001:db8::1]:1234", want: "[2001:db8::1]"},
		{name: "IPv6 without port", in: "2001:db8::1", want: "2001:db8::1"},
		{name: "bracketed IPv6 without port", in: "[2001:db8::1]", want: "[2001:db8::1]"},
		{name: "empty", in: "", want: ""},
		{name: "malformed host", in: "invalid-address:8080", want: "invalid-address:8080"},
		{name: "IPv6 zone with port", in: "[fe80::1%eth0]:80", want: "fe80::1%eth0"},
		{name: "IPv6 zone without port", in: "fe80::1%eth0", want: "fe80::1%eth0"},
	}

	for setterName, set := range setters {
		for _, tt := range tests {
			t.Run(setterName+"/"+tt.name, func(t *testing.T) {
				ctx := set(context.Background(), tt.in)
				got, ok := GetClientIP[int, testUser](ctx)
				if !ok || got != tt.want {
					t.Fatalf("GetClientIP = (%q, %v), want (%q, true)", got, ok, tt.want)
				}
			})
		}
	}
}

func TestEquivalentClientIPSocketAddressesReuseCachedLogger(t *testing.T) {
	setters := map[string]func(context.Context, string) context.Context{
		"WithClientIP": func(ctx context.Context, ip string) context.Context {
			return WithClientIP[int, testUser](ctx, ip)
		},
		"WithClientInfo": func(ctx context.Context, ip string) context.Context {
			return WithClientInfo[int, testUser](ctx, ip, "test-agent")
		},
	}

	for name, set := range setters {
		t.Run(name, func(t *testing.T) {
			base, logs := newObservedLogger()
			ctx := WithRequestLogger[int, testUser](context.Background(), NewRequestLoggerSource[int](base, nil))
			ctx = set(ctx, "192.0.2.1:1234")
			first, _ := GetLogger[int, testUser](ctx)

			ctx = set(ctx, "192.0.2.1:5678")
			second, _ := GetLogger[int, testUser](ctx)
			if second != first {
				t.Fatal("equivalent normalized client IP rebuilt the cached logger")
			}
			if got := logAndTake(t, second, logs, "normalized IP").ContextMap()[logkeys.ClientIP]; got != "192.0.2.1" {
				t.Fatalf("client_ip = %v, want 192.0.2.1", got)
			}
		})
	}
}
