package scontext

import (
	"context"
	"testing"
	"unsafe"
)

var contextSink context.Context
var wrapperSink *SRouterContext[int, testUser]

func TestContextSize(t *testing.T) {
	t.Logf("context bytes: int=%d string=%d uuid=%d", unsafe.Sizeof(SRouterContext[int, testUser]{}), unsafe.Sizeof(SRouterContext[string, testUser]{}), unsafe.Sizeof(SRouterContext[[16]byte, testUser]{}))
}

func BenchmarkContextCreate(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		wrapperSink = NewSRouterContext[int, testUser]()
	}
}

func BenchmarkContextCopy(b *testing.B) {
	ctx := WithUserID[int, testUser](context.Background(), 42)
	ctx = WithBuildID[int, testUser](ctx, "build")
	b.ReportAllocs()
	for b.Loop() {
		contextSink = CopySRouterContext[int, testUser](ctx, ctx)
	}
}

var correlationSink Correlation[int]

func TestCorrelationSize(t *testing.T) {
	t.Logf("correlation bytes: int=%d string=%d uuid=%d", unsafe.Sizeof(Correlation[int]{}), unsafe.Sizeof(Correlation[string]{}), unsafe.Sizeof(Correlation[[16]byte]{}))
}

func BenchmarkGetCorrelation(b *testing.B) {
	ctx := WithUserID[int, testUser](context.Background(), 42)
	ctx = WithTraceID[int, testUser](ctx, "trace")
	ctx = WithBuildID[int, testUser](ctx, "build")
	ctx = WithConfigID[int, testUser](ctx, "config")
	b.ReportAllocs()
	for b.Loop() {
		correlationSink, _ = GetCorrelation[int](ctx)
	}
}
