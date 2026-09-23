package scontext

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Any call, including GetDB, indicates that clearing exceeded its contract.
type untouchedTransaction struct{ t *testing.T }

func (tx *untouchedTransaction) Commit() error   { tx.t.Error("unexpected Commit"); return nil }
func (tx *untouchedTransaction) Rollback() error { tx.t.Error("unexpected Rollback"); return nil }
func (tx *untouchedTransaction) SavePoint(string) error {
	tx.t.Error("unexpected SavePoint")
	return nil
}
func (tx *untouchedTransaction) RollbackTo(string) error {
	tx.t.Error("unexpected RollbackTo")
	return nil
}
func (tx *untouchedTransaction) GetDB() *gorm.DB { tx.t.Error("unexpected GetDB"); return nil }

func TestClearTransaction(t *testing.T) {
	for _, state := range []string{"attached", "present nil", "absent", "no carrier"} {
		t.Run(state, func(t *testing.T) {
			ctx := context.Background()
			switch state {
			case "attached":
				ctx = WithTransaction[int, testUser](ctx, &untouchedTransaction{t})
			case "present nil":
				ctx = WithTransaction[int, testUser](ctx, nil)
				if tx, ok := GetTransaction[int](ctx); tx != nil || !ok {
					t.Fatal("nil transaction must remain explicitly present before clearing")
				}
			case "absent":
				_, ctx = EnsureSRouterContext[int, testUser](ctx)
			}
			for range 2 {
				if ClearTransaction[int, testUser](ctx) != ctx {
					t.Fatal("clear replaced the context")
				}
				if tx, ok := GetTransaction[int](ctx); tx != nil || ok {
					t.Fatalf("GetTransaction = (%v, %v), want (nil, false)", tx, ok)
				}
			}
			if state == "no carrier" {
				if _, ok := GetSRouterContext[int, testUser](ctx); ok {
					t.Fatal("clear allocated a carrier")
				}
			}
		})
	}
}

func TestClearTransactionMismatchedTypes(t *testing.T) {
	tx := &untouchedTransaction{t}
	ctx := WithTransaction[int, testUser](context.Background(), tx)
	for name, clear := range map[string]func(context.Context) context.Context{
		"user ID":     ClearTransaction[string, testUser],
		"user object": ClearTransaction[int, string],
	} {
		t.Run(name, func(t *testing.T) {
			if clear(ctx) != ctx {
				t.Fatal("mismatched clear replaced context")
			}
			if got, ok := GetTransaction[int](ctx); got != tx || !ok {
				t.Fatal("mismatched clear changed transaction")
			}
		})
	}
}

func TestTransactionIsolation(t *testing.T) {
	tx := &untouchedTransaction{t}
	parent := WithTransaction[int, testUser](context.Background(), tx)
	type key struct{}
	sibling := context.WithValue(parent, key{}, "sibling")
	child := CopySRouterContext[int, testUser](parent, parent)
	ClearTransaction[int, testUser](child)
	for _, ctx := range []context.Context{parent, sibling} {
		if got, ok := GetTransaction[int](ctx); got != tx || !ok {
			t.Fatal("clearing clone changed parent or sibling")
		}
	}
	if got, ok := GetTransaction[int](child); got != nil || ok {
		t.Fatal("clone still has transaction")
	}

	replacement := &untouchedTransaction{t}
	child = CopySRouterContext[int, testUser](parent, parent)
	WithTransaction[int, testUser](child, replacement)
	if got, ok := GetTransaction[int](child); got != replacement || !ok {
		t.Fatal("clone did not receive replacement")
	}
	for _, ctx := range []context.Context{parent, sibling} {
		if got, ok := GetTransaction[int](ctx); got != tx || !ok {
			t.Fatal("replacing clone changed parent or sibling")
		}
	}

	// Without cloning, derived contexts deliberately share the carrier.
	ClearTransaction[int, testUser](sibling)
	if got, ok := GetTransaction[int](parent); got != nil || ok {
		t.Fatal("clear did not mutate shared carrier")
	}
}

func TestClearTransactionPreservesContext(t *testing.T) {
	type key struct{}
	ctx, cancel := context.WithDeadline(createFullSRouterContext(), time.Now().Add(time.Hour))
	defer cancel()
	ctx = context.WithValue(ctx, key{}, "value")
	ctx = WithRequestLogger[int, testUser](ctx, NewRequestLoggerSource[int](zap.NewNop(), nil))
	logger, _ := GetLogger[int](ctx)
	correlation, _ := GetCorrelation[int](ctx)
	deadline, _ := ctx.Deadline()
	rc, _ := GetSRouterContext[int, testUser](ctx)
	version, loggerVersion := rc.logVersion, rc.loggerVersion

	child := CopySRouterContext[int, testUser](ctx, ctx)
	if ClearTransaction[int, testUser](child) != child {
		t.Fatal("clear replaced context")
	}
	childRC, _ := GetSRouterContext[int, testUser](child)
	if childRC.logVersion != version || childRC.loggerVersion != loggerVersion {
		t.Fatal("clear invalidated logger cache")
	}
	if got, ok := GetLogger[int](child); got != logger || !ok {
		t.Fatal("cached logger changed")
	}
	if got, ok := GetCorrelation[int](child); got != correlation || !ok {
		t.Fatal("correlation changed")
	}
	if got, ok := child.Deadline(); got != deadline || !ok || child.Done() != ctx.Done() || child.Value(key{}) != "value" {
		t.Fatal("context chain changed")
	}
	if got, ok := GetTransaction[int](child); got != nil || ok {
		t.Fatal("transaction not cleared")
	}
	// Restore only the transaction so the existing full-state checker can
	// verify every other stored value, including route, CORS, flags, and user.
	WithTransaction[int, testUser](child, &untouchedTransaction{t})
	verifyFullSRouterContext(t, child, "clear preserves state")
	cancel()
	if child.Err() != context.Canceled {
		t.Fatal("cancellation not preserved")
	}
}

func TestClearTransactionConcurrent(t *testing.T) {
	tx := &untouchedTransaction{t}
	ctx := WithTransaction[int, testUser](context.Background(), tx)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 100 {
				WithTransaction[int, testUser](ctx, tx)
				got, ok := GetTransaction[int](ctx)
				if (ok && got != tx) || (!ok && got != nil) {
					t.Error("inconsistent transaction and presence")
				}
				ClearTransaction[int, testUser](ctx)
			}
		})
	}
	wg.Wait()
	ClearTransaction[int, testUser](ctx)
	if got, ok := GetTransaction[int](ctx); got != nil || ok {
		t.Fatal("final clear failed")
	}
}
