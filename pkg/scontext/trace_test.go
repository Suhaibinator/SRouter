package scontext

import (
	"context"
	"sync"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestSetTraceIDReplacesAndInvalidatesLogger(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	ctx := SetTraceID[int, string](context.Background(), "first")
	ctx = WithRequestLogger[int, string](ctx, NewRequestLoggerSource[int](zap.New(core), nil))
	first, _ := GetLogger[int, string](ctx)
	ctx = WithTraceID[int, string](ctx, "preserved")
	preserved, _ := GetLogger[int, string](ctx)
	if preserved != first {
		t.Fatal("WithTraceID invalidated preserved ID")
	}
	if next := SetTraceID[int, string](ctx, "second"); next != ctx {
		t.Fatal("setter replaced shared context")
	}
	second, _ := GetLogger[int, string](ctx)
	first.Info("first")
	second.Info("second")
	if logs.All()[0].ContextMap()["trace_id"] != "first" || logs.All()[1].ContextMap()["trace_id"] != "second" {
		t.Fatal("logger snapshot mismatch")
	}
	SetTraceID[int, string](ctx, "")
	WithTraceID[int, string](ctx, "must-not-replace-empty")
	if got := GetTraceID[int, string](ctx); got != "" {
		t.Fatalf("empty set ID overwritten: %q", got)
	}
	empty, _ := GetLogger[int, string](ctx)
	empty.Info("empty")
	if _, ok := logs.All()[2].ContextMap()["trace_id"]; ok {
		t.Fatal("empty trace logged")
	}
}

func TestSetTraceIDConcurrent(t *testing.T) {
	ctx := SetTraceID[int, string](context.Background(), "start")
	ctx = WithRequestLogger[int, string](ctx, NewRequestLoggerSource[int](zap.NewNop(), nil))
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 100 {
				SetTraceID[int, string](ctx, "replacement")
				WithTraceID[int, string](ctx, "preserved")
				_ = GetTraceID[int, string](ctx)
				_, _ = GetLogger[int, string](ctx)
			}
		})
	}
	wg.Wait()
}
