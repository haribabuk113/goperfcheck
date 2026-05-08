package checker

import (
	"go/ast"
	"go/token"
)

// ObjectPoolChecker detects high-churn allocations that could be pooled with
// sync.Pool to reduce GC pressure.
//
// Rule: https://goperf.dev/01-common-patterns/object-pooling/
// Benchmark from site: pooled buffers achieve ~20x throughput improvement and
// zero allocations per operation versus one allocation per operation without pooling.
type ObjectPoolChecker struct{}

func (c *ObjectPoolChecker) Name() string { return "ObjectPool" }

// poolCandidates are pkg.Func calls that create reusable short-lived objects.
var poolCandidates = map[string]map[string]bool{
	"bytes":   {"NewBuffer": true, "NewBufferString": true},
	"strings": {"NewReader": true},
	"bufio":   {"NewWriter": true, "NewReader": true, "NewReadWriter": true},
	"json":    {"NewEncoder": true, "NewDecoder": true},
	"zip":     {"NewWriter": true},
	"gzip":    {"NewWriter": true},
}

func (c *ObjectPoolChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	var issues []Issue

	// Flag known poolable allocations inside loops.
	walkLoopBodies(file, func(body *ast.BlockStmt) {
		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			pkg, fn, ok := callName(call)
			if !ok {
				return true
			}
			if fns, hasPkg := poolCandidates[pkg]; hasPkg && fns[fn] {
				f, line, col := nodePos(fset, call)
				issues = append(issues, Issue{
					Checker:    c.Name(),
					File:       f,
					Line:       line,
					Column:     col,
					Severity:   SeverityWarning,
					Message:    pkg + "." + fn + "() called inside a loop — a new allocation is made every iteration",
					Rule:       "Object Pooling — https://goperf.dev/01-common-patterns/object-pooling/",
					Suggestion: "Declare a sync.Pool{New: func() any { return " + pkg + "." + fn + "(...) }}; call Get(), Reset(), use, then Put()",
					Benchmark:  "1 alloc/op → 0; ~2× faster per operation with sync.Pool reuse (Go 1.26 benchmark, see benchmarks/)",
				})
			}
			return true
		})
	})

	// Flag make([]byte, n) inside loops — a common pattern for scratch buffers.
	walkLoopBodies(file, func(body *ast.BlockStmt) {
		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			id, ok := call.Fun.(*ast.Ident)
			if !ok || id.Name != "make" || len(call.Args) < 2 {
				return true
			}
			arr, ok := call.Args[0].(*ast.ArrayType)
			if !ok || arr.Len != nil {
				return true
			}
			elt, ok := arr.Elt.(*ast.Ident)
			if ok && elt.Name == "byte" {
				f, line, col := nodePos(fset, call)
				issues = append(issues, Issue{
					Checker:    c.Name(),
					File:       f,
					Line:       line,
					Column:     col,
					Severity:   SeverityWarning,
					Message:    "make([]byte, n) inside a loop creates a new heap allocation every iteration",
					Rule:       "Object Pooling — https://goperf.dev/01-common-patterns/object-pooling/",
					Suggestion: "Use a sync.Pool to reuse byte slices; Get the slice, use it, then Put it back",
					Benchmark:  "1 alloc/op → 0; ~2× faster per operation with sync.Pool reuse (Go 1.26 benchmark, see benchmarks/)",
				})
			}
			return true
		})
	})

	return issues
}
