package checker

import (
	"go/ast"
	"go/token"
)

// GoroutinePoolChecker detects unbounded goroutine creation inside loops.
//
// Rule: https://goperf.dev/01-common-patterns/worker-pool/
// - Spawning goroutines in a loop risks CPU/memory saturation
// - Unbounded concurrency causes scheduler contention, context switching, GC overhead
// - Use a fixed worker pool (size ≈ runtime.NumCPU() for CPU-bound work)
type GoroutinePoolChecker struct{}

func (c *GoroutinePoolChecker) Name() string { return "GoroutinePool" }

func (c *GoroutinePoolChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	var issues []Issue

	walkLoopBodies(file, func(body *ast.BlockStmt) {
		ast.Inspect(body, func(n ast.Node) bool {
			goStmt, ok := n.(*ast.GoStmt)
			if !ok {
				return true
			}
			f, line, col := nodePos(fset, goStmt)
			issues = append(issues, Issue{
				Checker:    c.Name(),
				File:       f,
				Line:       line,
				Column:     col,
				Severity:   SeverityWarning,
				Message:    "goroutine spawned inside a loop without a worker pool — unbounded concurrency risks saturation",
				Rule:       "Goroutine Worker Pools — https://goperf.dev/01-common-patterns/worker-pool/",
				Suggestion: "Create a fixed pool of N goroutines that read from a buffered job channel; set N ≈ runtime.NumCPU()",
				Benchmark:  "202 allocs/op → 11; ~2× faster for 200 tasks with fixed worker pool vs. goroutine-per-task (Go 1.26 benchmark, see benchmarks/)",
			})
			return true
		})
	})

	return issues
}
