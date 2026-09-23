// Run with go run . from examples/transaction-context.
package main

import (
	"context"
	"fmt"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"gorm.io/gorm"
)

// exampleTransaction implements the interface without needing a database.
type exampleTransaction struct{}

func (*exampleTransaction) Commit() error           { return nil }
func (*exampleTransaction) Rollback() error         { return nil }
func (*exampleTransaction) SavePoint(string) error  { return nil }
func (*exampleTransaction) RollbackTo(string) error { return nil }
func (*exampleTransaction) GetDB() *gorm.DB         { return nil }

func main() {
	parent := scontext.WithTransaction[int, string](context.Background(), &exampleTransaction{})
	child := scontext.CopySRouterContext[int, string](parent, parent)
	child = scontext.ClearTransaction[int, string](child)

	_, parentHasTransaction := scontext.GetTransaction[int](parent)
	_, childHasTransaction := scontext.GetTransaction[int](child)
	fmt.Println("parent has transaction:", parentHasTransaction)
	fmt.Println("child has transaction:", childHasTransaction)
}
