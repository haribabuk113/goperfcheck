// Rule: call wg.Add before the go statement, never inside the goroutine.
package checker

import (
	"go/ast"
	"go/token"
)

type WaitGroupMisuseChecker struct{}

func (WaitGroupMisuseChecker) Name() string { return "WaitGroupMisuse" }

// Check finds wg.Add(n) calls inside goroutine function literals.
// Calling Add inside the goroutine is a race: Wait() can return before
// the counter is incremented if the scheduler runs the waiting goroutine first.
//
// Heuristic: flags expr.Add(intLiteral) where expr is a plain identifier,
// narrowing to the WaitGroup.Add(1) pattern while avoiding false positives
// from db.Add(), list.Add(n) etc. that pass non-identifier receivers.
func (WaitGroupMisuseChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	var issues []Issue

	ast.Inspect(file, func(n ast.Node) bool {
		goStmt, ok := n.(*ast.GoStmt)
		if !ok {
			return true
		}
		lit, ok := goStmt.Call.Fun.(*ast.FuncLit)
		if !ok || lit.Body == nil {
			return true
		}
		ast.Inspect(lit.Body, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Add" {
				return true
			}
			// Require a plain identifier receiver (wg.Add, not s.wg.Add)
			// and a single integer-literal argument to reduce false positives.
			if _, isIdent := sel.X.(*ast.Ident); !isIdent {
				return true
			}
			if len(call.Args) != 1 {
				return true
			}
			if arg, ok := call.Args[0].(*ast.BasicLit); !ok || arg.Kind != token.INT {
				return true
			}
			f, l, c := nodePos(fset, call)
			issues = append(issues, Issue{
				Checker:    "WaitGroupMisuse",
				File:       f,
				Line:       l,
				Column:     c,
				Severity:   SeverityError,
				Message:    "wg.Add() called inside the goroutine — creates a race where Wait() may return before the counter is incremented",
				Rule:       "sync.WaitGroup usage — https://goperf.dev/01-common-patterns/goroutines/",
				Suggestion: "Call wg.Add(1) before the go statement so the counter is incremented before any Wait() can unblock",
				Benchmark:  "correctness rule — race detector flags this; under load, Wait() unblocks before work completes, causing data loss or nil-pointer panics",
			})
			return true
		})
		return true
	})

	return issues
}
