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

// populateAll writes request values through the full [T, U] setters.
func populateAll[T comparable, U any](t *testing.T, id T, tx DatabaseTransaction, handlerErr error) context.Context {
	t.Helper()
	ctx := context.Background()
	ctx = WithRequestLogger[T, U](ctx, NewRequestLoggerSource[T](zap.NewNop(), nil))
	ctx = WithUserID[T, U](ctx, id)
	ctx = WithBuildID[T, U](ctx, "build-1")
	ctx = WithConfigID[T, U](ctx, "config-1")
	ctx = WithFlag[T, U](ctx, "feature", true)
	ctx = WithClientInfo[T, U](ctx, "192.0.2.1", "agent/1")
	ctx = WithTransaction[T, U](ctx, tx)
	ctx = WithTraceID[T, U](ctx, "trace-1")
	ctx = WithCORSInfo[T, U](ctx, "https://example.com", true)
	ctx = WithCORSRequestedHeaders[T, U](ctx, "X-Test")
	ctx = WithHandlerError[T, U](ctx, handlerErr)
	return ctx
}

// populateWithID binds the ID while preserving a common fixture signature.
func populateWithID[T comparable, U any](id T) func(*testing.T, DatabaseTransaction, error) context.Context {
	return func(t *testing.T, tx DatabaseTransaction, err error) context.Context {
		return populateAll[T, U](t, id, tx, err)
	}
}

type namedReaderID string

// TestGettersIgnoreUserTypes verifies every non-generic getter across carriers.
func TestGettersIgnoreUserTypes(t *testing.T) {
	cases := map[string]func(*testing.T, DatabaseTransaction, error) context.Context{
		"uint64/struct":    populateWithID[uint64, readerStructUser](42),
		"uint64/pointers":  populateWithID[uint64, readerPointerUser](42),
		"uint64/interface": populateWithID[uint64, readerInterfaceUser](42),
		"uint64/any":       populateWithID[uint64, any](42),
		"string/struct":    populateWithID[string, readerStructUser]("user-42"),
		"string/pointers":  populateWithID[string, readerPointerUser]("user-42"),
		"string/interface": populateWithID[string, readerInterfaceUser]("user-42"),
		"string/any":       populateWithID[string, any]("user-42"),
		"named/struct":     populateWithID[namedReaderID, readerStructUser]("user-42"),
	}
	for name, populate := range cases {
		t.Run(name, func(t *testing.T) {
			tx := &mockTransaction{}
			handlerErr := errors.New("handler failed")
			ctx := populate(t, tx, handlerErr)

			if v, ok := GetBuildID(ctx); !ok || v != "build-1" {
				t.Errorf("GetBuildID = (%q, %v), want (build-1, true)", v, ok)
			}
			if v, ok := GetConfigID(ctx); !ok || v != "config-1" {
				t.Errorf("GetConfigID = (%q, %v), want (config-1, true)", v, ok)
			}
			if v, ok := GetFlag(ctx, "feature"); !ok || !v {
				t.Errorf("GetFlag = (%v, %v), want (true, true)", v, ok)
			}
			if v, ok := GetClientIP(ctx); !ok || v != "192.0.2.1" {
				t.Errorf("GetClientIP = (%q, %v), want (192.0.2.1, true)", v, ok)
			}
			if v, ok := GetUserAgent(ctx); !ok || v != "agent/1" {
				t.Errorf("GetUserAgent = (%q, %v), want (agent/1, true)", v, ok)
			}
			if v, ok := GetTransaction(ctx); !ok || v != tx {
				t.Errorf("GetTransaction = (%v, %v), want (%v, true)", v, ok, tx)
			}
			if v := GetTraceID(ctx); v != "trace-1" {
				t.Errorf("GetTraceID = %q, want trace-1", v)
			}
			if origin, creds, ok := GetCORSInfo(ctx); !ok || origin != "https://example.com" || !creds {
				t.Errorf("GetCORSInfo = (%q, %v, %v), want (https://example.com, true, true)", origin, creds, ok)
			}
			if v, ok := GetCORSRequestedHeaders(ctx); !ok || v != "X-Test" {
				t.Errorf("GetCORSRequestedHeaders = (%q, %v), want (X-Test, true)", v, ok)
			}
			if v, ok := GetHandlerError(ctx); !ok || v != handlerErr {
				t.Errorf("GetHandlerError = (%v, %v), want (%v, true)", v, ok, handlerErr)
			}
			if logger, ok := GetLogger(ctx); !ok || logger == nil {
				t.Errorf("GetLogger = (%v, %v), want a logger", logger, ok)
			}
		})
	}
}

// Typed getters still require the matching type when their result depends on it.
func TestTypedGettersMatchUserTypes(t *testing.T) {
	ctx := populateAll[uint64, readerStructUser](t, 42, &mockTransaction{}, errors.New("handler failed"))
	user := &readerStructUser{Name: "user"}
	ctx = WithUser[uint64, readerStructUser](ctx, user)
	if id, ok := GetUserID[uint64](ctx); !ok || id != 42 {
		t.Fatal("matching user ID not found")
	}
	if c, ok := GetCorrelation[uint64](ctx); !ok || c.UserID != 42 || c.TraceID != "trace-1" {
		t.Fatal("matching correlation not found")
	}
	if got, ok := GetUser[uint64, readerStructUser](ctx); !ok || got != user {
		t.Fatal("matching user not found")
	}
	if _, ok := GetSRouterContext[uint64, readerStructUser](ctx); !ok {
		t.Fatal("matching carrier not found")
	}
	if _, ok := GetUserID[string](ctx); ok {
		t.Fatal("mismatched user ID found")
	}
	if _, ok := GetCorrelation[string](ctx); ok {
		t.Fatal("mismatched correlation found")
	}
	if _, ok := GetUser[uint64, string](ctx); ok {
		t.Fatal("mismatched user object found")
	}
	if _, ok := GetUser[string, readerStructUser](ctx); ok {
		t.Fatal("user found with mismatched user ID type")
	}
	if _, ok := GetSRouterContext[uint64, string](ctx); ok {
		t.Fatal("carrier found with mismatched user object type")
	}
	if _, ok := GetSRouterContext[string, readerStructUser](ctx); ok {
		t.Fatal("carrier found with mismatched user ID type")
	}
}
