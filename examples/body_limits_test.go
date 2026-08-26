package examples_test

import (
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"testing"
)

func TestBodyDecodingExamplesConfigurePositiveGlobalLimit(t *testing.T) {
	t.Parallel()

	files := []string{
		"codec/main.go",
		"handler-error-middleware/main.go",
		"nested-subrouters/main.go",
		"rate-limiting/main.go",
		"subrouter-generic-routes/main.go",
	}

	for _, name := range files {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, filepath.FromSlash(name), nil, 0)
			if err != nil {
				t.Fatal(err)
			}

			foundRouterConfig := false
			ast.Inspect(file, func(node ast.Node) bool {
				literal, ok := node.(*ast.CompositeLit)
				if !ok || !isRouterConfig(literal.Type) {
					return true
				}

				foundRouterConfig = true
				for _, element := range literal.Elts {
					field, ok := element.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key, keyOK := field.Key.(*ast.Ident)
					if !keyOK || key.Name != "GlobalMaxBodySize" {
						continue
					}

					expression := types.ExprString(field.Value)
					value, err := types.Eval(fset, nil, token.NoPos, expression)
					if err != nil {
						t.Fatalf("evaluate GlobalMaxBodySize %q: %v", expression, err)
					}
					if amount, ok := constant.Int64Val(value.Value); !ok || amount <= 0 {
						t.Fatalf("GlobalMaxBodySize must be a positive integer, got %s", expression)
					}
					return false
				}

				t.Error("RouterConfig must set GlobalMaxBodySize for body-decoding routes")
				return false
			})

			if !foundRouterConfig {
				t.Fatal("RouterConfig not found")
			}
		})
	}
}

func isRouterConfig(expr ast.Expr) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "RouterConfig" {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	return ok && packageName.Name == "router"
}
