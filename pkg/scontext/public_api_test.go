package scontext_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
)

func TestSRouterContextFieldsArePrivate(t *testing.T) {
	typ := reflect.TypeFor[scontext.SRouterContext[int, string]]()
	for i := range typ.NumField() {
		if field := typ.Field(i); field.IsExported() {
			t.Errorf("SRouterContext exposes field %s", field.Name)
		}
	}
	// The exported type's zero value remains usable through public helpers.
	ctx := scontext.WithSRouterContext(context.Background(), &scontext.SRouterContext[int, string]{})
	ctx = scontext.WithUserID[int, string](ctx, 42)
	if id, ok := scontext.GetUserID[int](ctx); id != 42 || !ok {
		t.Fatal("zero-value carrier cannot be used through helpers")
	}
}

func ExampleClearTransaction() {
	// Even nil can be an explicitly present transaction.
	parent := scontext.WithTransaction[int, string](context.Background(), nil)
	child := scontext.CopySRouterContext[int, string](parent, parent)
	child = scontext.ClearTransaction[int, string](child)
	_, parentHasTransaction := scontext.GetTransaction(parent)
	_, childHasTransaction := scontext.GetTransaction(child)
	fmt.Println("parent:", parentHasTransaction)
	fmt.Println("child:", childHasTransaction)
	// Output:
	// parent: true
	// child: false
}
