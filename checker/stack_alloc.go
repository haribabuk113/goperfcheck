package checker

import (
	"go/ast"
	"go/token"
)

// StackAllocChecker flags allocation patterns that force variables to escape
// to the heap when stack allocation would be possible.
//
// Rule: https://goperf.dev/01-common-patterns/stack-alloc/
// - Stack allocations are garbage-free and fast
// - Heap escapes incur GC overhead
// - Variables that don't outlive their function can stay on the stack
type StackAllocChecker struct{}

func (c *StackAllocChecker) Name() string { return "StackAlloc" }

// primitiveTypes are small types where returning by value avoids escape.
var primitiveTypes = map[string]bool{
	"int":    true,
	"int8":   true,
	"int16":  true,
	"int32":  true,
	"int64":  true,
	"uint":   true,
	"uint8":  true,
	"uint16": true,
	"uint32": true,
	"uint64": true,
	"float32": true,
	"float64": true,
	"bool":   true,
	"byte":   true,
	"rune":   true,
}

func (c *StackAllocChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	var issues []Issue

	// 1. new(primitiveType) — always heap; use value semantics instead
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok || id.Name != "new" || len(call.Args) != 1 {
			return true
		}
		argID, ok := call.Args[0].(*ast.Ident)
		if ok && primitiveTypes[argID.Name] {
			f, line, col := nodePos(fset, call)
			issues = append(issues, Issue{
				Checker:    c.Name(),
				File:       f,
				Line:       line,
				Column:     col,
				Severity:   SeverityWarning,
				Message:    "new(" + argID.Name + ") allocates on the heap — use var x " + argID.Name + " for stack allocation",
				Rule:       "Stack Allocations — https://goperf.dev/01-common-patterns/stack-alloc/",
				Suggestion: "Use value semantics: var x T or x := T(0) to keep the variable on the stack",
			})
		}
		return true
	})

	// 2. Return of &localVar for primitive types (forces escape)
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}

		// Collect local primitive var names
		localPrimitives := map[string]bool{}
		ast.Inspect(fn.Body, func(inner ast.Node) bool {
			vs, ok := inner.(*ast.ValueSpec)
			if ok && vs.Type != nil {
				if id, ok := vs.Type.(*ast.Ident); ok && primitiveTypes[id.Name] {
					for _, name := range vs.Names {
						localPrimitives[name.Name] = true
					}
				}
			}
			return true
		})

		// Check return statements for &localPrimitive
		ast.Inspect(fn.Body, func(inner ast.Node) bool {
			ret, ok := inner.(*ast.ReturnStmt)
			if !ok {
				return true
			}
			for _, result := range ret.Results {
				unary, ok := result.(*ast.UnaryExpr)
				if !ok {
					continue
				}
				if unary.Op.String() != "&" {
					continue
				}
				if id, ok := unary.X.(*ast.Ident); ok && localPrimitives[id.Name] {
					f, line, col := nodePos(fset, unary)
					issues = append(issues, Issue{
						Checker:    c.Name(),
						File:       f,
						Line:       line,
						Column:     col,
						Severity:   SeverityInfo,
						Message:    "returning &" + id.Name + " (a local primitive) forces heap allocation — return by value instead",
						Rule:       "Stack Allocations — https://goperf.dev/01-common-patterns/stack-alloc/",
						Suggestion: "Return the value directly unless the caller genuinely needs a pointer",
					})
				}
			}
			return true
		})
		return true
	})

	return issues
}
