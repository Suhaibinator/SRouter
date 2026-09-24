// Run with go run . from examples/identity-context.
package main

import (
	"context"
	"fmt"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
)

type User struct{ Name string }

func main() {
	parent := scontext.WithUserID[int, User](context.Background(), 42)
	parent = scontext.WithUser[int, User](parent, &User{Name: "Alice"})
	child := scontext.CopySRouterContext[int, User](parent, parent)
	child = scontext.ClearIdentity[int, User](child)
	_, parentID := scontext.GetUserID[int](parent)
	_, parentUser := scontext.GetUser[int, User](parent)
	_, childID := scontext.GetUserID[int](child)
	_, childUser := scontext.GetUser[int, User](child)
	fmt.Printf("parent: user ID present=%t, user present=%t\n", parentID, parentUser)
	fmt.Printf("child: user ID present=%t, user present=%t\n", childID, childUser)
}
