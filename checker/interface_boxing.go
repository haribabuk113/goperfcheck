package checker

import (
	"fmt"
	"go/ast"
	"go/token"
)

// InterfaceBoxingChecker detects use of empty interface{} that causes heap boxing.
//
// Rule: https://goperf.dev/01-common-patterns/interface-boxing/
// - Large structs passed directly to interface{} parameters cause heap allocation and copying
// - Benchmarks show ~19% performance degradation for 4KB structs
// - Slices of interface{} individually box each element
type InterfaceBoxingChecker struct{}

func (c *InterfaceBoxingChecker) Name() string { return "InterfaceBoxing" }

func isEmptyInterface(expr ast.Expr) bool {
	iface, ok := expr.(*ast.InterfaceType)
	return ok && (iface.Methods == nil || len(iface.Methods.List) == 0)
}

func (c *InterfaceBoxingChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	var issues []Issue

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {

		// []interface{} slice type
		case *ast.ArrayType:
			if node.Len == nil && isEmptyInterface(node.Elt) {
				f, line, col := nodePos(fset, node)
				issues = append(issues, Issue{
					Checker:    c.Name(),
					File:       f,
					Line:       line,
					Column:     col,
					Severity:   SeverityWarning,
					Message:    "[]interface{} detected — each element is individually heap-boxed",
					Rule:       "Interface Boxing — https://goperf.dev/01-common-patterns/interface-boxing/",
					Suggestion: "Use a typed slice []ConcreteType or generics instead",
				})
			}

		// Function/method parameters of type interface{}
		case *ast.FuncDecl:
			if node.Type == nil || node.Type.Params == nil {
				return true
			}
			for _, param := range node.Type.Params.List {
				if isEmptyInterface(param.Type) {
					f, line, col := nodePos(fset, param)
					pname := "param"
					if len(param.Names) > 0 {
						pname = param.Names[0].Name
					}
					issues = append(issues, Issue{
						Checker:    c.Name(),
						File:       f,
						Line:       line,
						Column:     col,
						Severity:   SeverityInfo,
						Message:    fmt.Sprintf("function %q parameter %q is interface{} — all arguments will be heap-boxed", node.Name.Name, pname),
						Rule:       "Interface Boxing — https://goperf.dev/01-common-patterns/interface-boxing/",
						Suggestion: "Use a concrete type, named interface with methods, or generics instead",
					})
				}
			}

		// map[K]interface{} — values are boxed
		case *ast.MapType:
			if isEmptyInterface(node.Value) {
				f, line, col := nodePos(fset, node)
				issues = append(issues, Issue{
					Checker:    c.Name(),
					File:       f,
					Line:       line,
					Column:     col,
					Severity:   SeverityInfo,
					Message:    "map[K]interface{} — all values are heap-boxed; prefer map[K]ConcreteType",
					Rule:       "Interface Boxing — https://goperf.dev/01-common-patterns/interface-boxing/",
					Suggestion: "Use a typed map or a struct if the key set is fixed",
				})
			}
		}
		return true
	})

	return issues
}
