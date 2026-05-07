// Rule: cache time.Now() before a loop when the same timestamp is acceptable.
package checker

import (
	"go/ast"
	"go/token"
)

type TimeNowLoopChecker struct{}

func (TimeNowLoopChecker) Name() string { return "TimeNowLoop" }

func (TimeNowLoopChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	var issues []Issue

	// checkBody walks a loop body for time.Now() calls, stopping at nested loops
	// so each call is attributed to its innermost enclosing loop.
	checkBody := func(body *ast.BlockStmt) {
		ast.Inspect(body, func(n ast.Node) bool {
			switch n.(type) {
			case *ast.ForStmt, *ast.RangeStmt:
				return false // nested loop — outer walker handles it
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			pkg, name, ok := callName(call)
			if !ok || pkg != "time" || name != "Now" {
				return true
			}
			f, l, c := nodePos(fset, call)
			issues = append(issues, Issue{
				Checker:    "TimeNowLoop",
				File:       f,
				Line:       l,
				Column:     c,
				Severity:   SeverityInfo,
				Message:    "time.Now() called inside a loop — each call is a syscall",
				Rule:       "Cache time.Now() — https://goperf.dev/01-common-patterns/time/",
				Suggestion: "Cache time.Now() in a variable before the loop if the same timestamp is acceptable across iterations",
			})
			return true
		})
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.ForStmt:
			if s.Body != nil {
				checkBody(s.Body)
			}
		case *ast.RangeStmt:
			if s.Body != nil {
				checkBody(s.Body)
			}
		}
		return true
	})

	return issues
}
