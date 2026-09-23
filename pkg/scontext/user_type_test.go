package scontext

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
)

type readerStructUser struct{ Name string }

type readerPointerUser struct {
	Roles map[string][]string
	Next  *readerPointerUser
}

type readerInterfaceUser interface{ ID() string }

// populateAll writes every value that a [T]-only getter reads through the
// setters for the context's full [T, U] type.
func populateAll[U any](t *testing.T, tx DatabaseTransaction, handlerErr error) context.Context {
	t.Helper()
	ctx := context.Background()
	ctx = WithRequestLogger[uint64, U](ctx, NewRequestLoggerSource[uint64](zap.NewNop(), nil))
	ctx = WithUserID[uint64, U](ctx, 42)
	ctx = WithBuildID[uint64, U](ctx, "build-1")
	ctx = WithConfigID[uint64, U](ctx, "config-1")
	ctx = WithFlag[uint64, U](ctx, "feature", true)
	ctx = WithClientInfo[uint64, U](ctx, "192.0.2.1", "agent/1")
	ctx = WithTransaction[uint64, U](ctx, tx)
	ctx = WithTraceID[uint64, U](ctx, "trace-1")
	ctx = WithCORSInfo[uint64, U](ctx, "https://example.com", true)
	ctx = WithCORSRequestedHeaders[uint64, U](ctx, "X-Test")
	ctx = WithHandlerError[uint64, U](ctx, handlerErr)
	return ctx
}

// TestGettersIgnoreUserType verifies that getters parameterized only by the
// user ID type read a wrapper created with any user object type.
func TestGettersIgnoreUserType(t *testing.T) {
	cases := map[string]func(*testing.T, DatabaseTransaction, error) context.Context{
		"struct":    populateAll[readerStructUser],
		"pointers":  populateAll[readerPointerUser],
		"interface": populateAll[readerInterfaceUser],
		"any":       populateAll[any],
	}
	for name, populate := range cases {
		t.Run(name, func(t *testing.T) {
			tx := &mockTransaction{}
			handlerErr := errors.New("handler failed")
			ctx := populate(t, tx, handlerErr)

			if id, ok := GetUserID[uint64](ctx); !ok || id != 42 {
				t.Errorf("GetUserID = (%d, %v), want (42, true)", id, ok)
			}
			if v, ok := GetBuildID[uint64](ctx); !ok || v != "build-1" {
				t.Errorf("GetBuildID = (%q, %v), want (build-1, true)", v, ok)
			}
			if v, ok := GetConfigID[uint64](ctx); !ok || v != "config-1" {
				t.Errorf("GetConfigID = (%q, %v), want (config-1, true)", v, ok)
			}
			if v, ok := GetFlag[uint64](ctx, "feature"); !ok || !v {
				t.Errorf("GetFlag = (%v, %v), want (true, true)", v, ok)
			}
			if v, ok := GetClientIP[uint64](ctx); !ok || v != "192.0.2.1" {
				t.Errorf("GetClientIP = (%q, %v), want (192.0.2.1, true)", v, ok)
			}
			if v, ok := GetUserAgent[uint64](ctx); !ok || v != "agent/1" {
				t.Errorf("GetUserAgent = (%q, %v), want (agent/1, true)", v, ok)
			}
			if v, ok := GetTransaction[uint64](ctx); !ok || v != tx {
				t.Errorf("GetTransaction = (%v, %v), want (%v, true)", v, ok, tx)
			}
			if v := GetTraceID[uint64](ctx); v != "trace-1" {
				t.Errorf("GetTraceID = %q, want trace-1", v)
			}
			if c, ok := GetCorrelation[uint64](ctx); !ok || c.UserID != 42 || c.TraceID != "trace-1" {
				t.Errorf("GetCorrelation = (%+v, %v), want user 42 and trace-1", c, ok)
			}
			if origin, creds, ok := GetCORSInfo[uint64](ctx); !ok || origin != "https://example.com" || !creds {
				t.Errorf("GetCORSInfo = (%q, %v, %v), want (https://example.com, true, true)", origin, creds, ok)
			}
			if v, ok := GetCORSRequestedHeaders[uint64](ctx); !ok || v != "X-Test" {
				t.Errorf("GetCORSRequestedHeaders = (%q, %v), want (X-Test, true)", v, ok)
			}
			if v, ok := GetHandlerError[uint64](ctx); !ok || v != handlerErr {
				t.Errorf("GetHandlerError = (%v, %v), want (%v, true)", v, ok, handlerErr)
			}
			if logger, ok := GetLogger[uint64](ctx); !ok || logger == nil {
				t.Errorf("GetLogger = (%v, %v), want a logger", logger, ok)
			}
		})
	}
}

// TestGettersWithMismatchedUserIDType documents that a wrapper created with a
// different user ID type is invisible to the getters, as it was when getters
// also named the user object type.
func TestGettersWithMismatchedUserIDType(t *testing.T) {
	ctx := populateAll[readerStructUser](t, &mockTransaction{}, errors.New("handler failed"))

	if _, ok := GetUserID[string](ctx); ok {
		t.Error("GetUserID[string] found a uint64 wrapper")
	}
	if _, ok := GetBuildID[string](ctx); ok {
		t.Error("GetBuildID[string] found a uint64 wrapper")
	}
	if _, ok := GetTransaction[string](ctx); ok {
		t.Error("GetTransaction[string] found a uint64 wrapper")
	}
	if v := GetTraceID[string](ctx); v != "" {
		t.Errorf("GetTraceID[string] = %q, want empty", v)
	}
	if _, ok := GetCorrelation[string](ctx); ok {
		t.Error("GetCorrelation[string] found a uint64 wrapper")
	}
	if _, ok := GetLogger[string](ctx); ok {
		t.Error("GetLogger[string] found a uint64 wrapper")
	}
}
