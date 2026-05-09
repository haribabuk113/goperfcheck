package checker

import (
	"go/ast"
	"go/token"
)

// DeferInLoopChecker detects defer statements inside for/range loops.
//
// Rule: https://goperf.dev/01-common-patterns/defer/
// - defer inside a loop allocates a closure per iteration on the heap
// - defers fire at function return, not at end of loop iteration — a common bug
// - for N iterations the cleanup is delayed until function exit, holding resources
type DeferInLoopChecker struct{}

func (c *DeferInLoopChecker) Name() string { return "DeferInLoop" }

func (c *DeferInLoopChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	var issues []Issue
	v := &deferInLoopVisitor{fset: fset, checker: c.Name()}
	ast.Walk(v, file)
	issues = append(issues, v.issues...)
	return issues
}

type deferInLoopVisitor struct {
	fset    *token.FileSet
	checker string
	inLoop  bool
	issues  []Issue
}

func (v *deferInLoopVisitor) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return nil
	}
	switch s := n.(type) {
	case *ast.FuncLit:
		// A defer inside a goroutine/closure is a separate function scope;
		// resets the loop context so inner loops are tracked independently.
		child := &deferInLoopVisitor{fset: v.fset, checker: v.checker, inLoop: false}
		ast.Walk(child, s.Body)
		v.issues = append(v.issues, child.issues...)
		return nil
	case *ast.ForStmt:
		child := &deferInLoopVisitor{fset: v.fset, checker: v.checker, inLoop: true}
		ast.Walk(child, s.Body)
		v.issues = append(v.issues, child.issues...)
		return nil
	case *ast.RangeStmt:
		child := &deferInLoopVisitor{fset: v.fset, checker: v.checker, inLoop: true}
		ast.Walk(child, s.Body)
		v.issues = append(v.issues, child.issues...)
		return nil
	case *ast.DeferStmt:
		if v.inLoop {
			f, line, col := nodePos(v.fset, s)
			v.issues = append(v.issues, Issue{
				Checker:    v.checker,
				File:       f,
				Line:       line,
				Column:     col,
				Severity:   SeverityWarning,
				Message:    "defer inside a loop allocates a closure per iteration and fires at function return, not loop iteration end",
				Rule:       "Defer in Loops — https://goperf.dev/01-common-patterns/defer/",
				Suggestion: "Move cleanup into an anonymous function called immediately (func() { defer f.Close(); ... }()) or track resources in a slice and clean up after the loop",
				Benchmark:  "defer in loop: ~130 ns/op, 1 alloc/op per iter; manual cleanup: ~6 ns/op, 0 allocs — ~20× faster for 1000-iteration loops (Go 1.26 benchmark, see benchmarks/)",
			})
		}
	}
	return v
}
