package checker

import (
	"fmt"
	"go/ast"
	"go/token"
)

// MemPreallocChecker detects slice/map allocations without capacity hints.
//
// Rule: https://goperf.dev/01-common-patterns/mem-prealloc/
//   - append() in a loop without a preallocated capacity causes repeated
//     reallocations (~4x slower, ~19x more allocations in benchmarks).
//   - make(map[K]V) without a size hint causes rehashing as the map grows.
type MemPreallocChecker struct{}

func (c *MemPreallocChecker) Name() string { return "MemPrealloc" }

func (c *MemPreallocChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	var issues []Issue

	// 1. append() calls inside for / range loops.
	walkLoopBodies(file, func(body *ast.BlockStmt) {
		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			id, ok := call.Fun.(*ast.Ident)
			if !ok || id.Name != "append" {
				return true
			}
			f, line, col := nodePos(fset, call)
			issues = append(issues, Issue{
				Checker:    c.Name(),
				File:       f,
				Line:       line,
				Column:     col,
				Severity:   SeverityWarning,
				Message:    "append() inside a loop — repeated reallocations occur when the backing array runs out of capacity",
				Rule:       "Memory Preallocation — https://goperf.dev/01-common-patterns/mem-prealloc/",
				Suggestion: "Before the loop use make([]T, 0, expectedLen) or make([]T, n) with index assignment",
			})
			return true
		})
	})

	// 2. make(map[K]V) without a capacity hint.
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok || id.Name != "make" || len(call.Args) == 0 {
			return true
		}
		if _, isMap := call.Args[0].(*ast.MapType); isMap && len(call.Args) == 1 {
			f, line, col := nodePos(fset, call)
			issues = append(issues, Issue{
				Checker:    c.Name(),
				File:       f,
				Line:       line,
				Column:     col,
				Severity:   SeverityInfo,
				Message:    fmt.Sprintf("make(map) on line %d has no capacity hint — map will rehash every time it doubles in size", line),
				Rule:       "Memory Preallocation — https://goperf.dev/01-common-patterns/mem-prealloc/",
				Suggestion: "Use make(map[K]V, expectedSize) to avoid rehashing overhead",
			})
		}
		return true
	})

	return issues
}
