package checker

import (
	"fmt"
	"go/ast"
	"go/token"
)

// ContextMisuseChecker flags incorrect context usage patterns.
//
// Rule: https://goperf.dev/01-common-patterns/context/
// - Never store context.Context in struct fields (must pass as first parameter)
// - context must flow explicitly through call chains for proper cancellation/timeout
// - Storing context causes stale contexts to be reused unintentionally
type ContextMisuseChecker struct{}

func (c *ContextMisuseChecker) Name() string { return "ContextMisuse" }

func isContextType(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "context" && sel.Sel.Name == "Context"
}

func (c *ContextMisuseChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	var issues []Issue

	// Detect context.Context stored as struct fields
	ast.Inspect(file, func(n ast.Node) bool {
		st, ok := n.(*ast.StructType)
		if !ok || st.Fields == nil {
			return true
		}
		for _, field := range st.Fields.List {
			if isContextType(field.Type) {
				f, line, col := nodePos(fset, field)
				fname := "field"
				if len(field.Names) > 0 {
					fname = field.Names[0].Name
				}
				issues = append(issues, Issue{
					Checker:    c.Name(),
					File:       f,
					Line:       line,
					Column:     col,
					Severity:   SeverityError,
					Message:    fmt.Sprintf("context.Context stored in struct field %q — contexts must never be stored in structs", fname),
					Rule:       "Efficient Context Management — https://goperf.dev/01-common-patterns/context/",
					Suggestion: "Pass context.Context as the first parameter to every function that needs it",
					Benchmark:  "correctness rule — stored context outlives the request scope, causing goroutine leaks and stale cancellation signals",
				})
			}
		}
		return true
	})

	return issues
}
