package checker

import (
	"go/ast"
	"go/token"
)

// ZeroCopyChecker detects patterns that introduce unnecessary data copies.
//
// Rule: https://goperf.dev/01-common-patterns/zero-copy/
// - append([]byte{}, src...) creates a full copy
// - copy() inside loops should use io.CopyBuffer with a reused buffer
// - Slice reslicing ([a:b]) is zero-copy
type ZeroCopyChecker struct{}

func (c *ZeroCopyChecker) Name() string { return "ZeroCopy" }

func (c *ZeroCopyChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	var issues []Issue

	// 1. append([]byte{}, src...) creates an unnecessary copy
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok || id.Name != "append" || len(call.Args) < 2 {
			return true
		}
		// Check if first arg is []byte{}
		lit, ok := call.Args[0].(*ast.CompositeLit)
		if !ok {
			return true
		}
		arr, ok := lit.Type.(*ast.ArrayType)
		if !ok || arr.Len != nil || len(lit.Elts) > 0 {
			return true
		}
		elt, ok := arr.Elt.(*ast.Ident)
		if !ok || elt.Name != "byte" {
			return true
		}

		f, line, col := nodePos(fset, call)
		issues = append(issues, Issue{
			Checker:    c.Name(),
			File:       f,
			Line:       line,
			Column:     col,
			Severity:   SeverityWarning,
			Message:    "append([]byte{}, src...) allocates a new buffer and copies all bytes",
			Rule:       "Zero-Copy Techniques — https://goperf.dev/01-common-patterns/zero-copy/",
			Suggestion: "Use a slice reference (src[a:b]) for reads; copy() only when isolation is required",
		})
		return true
	})

	// 2. copy() inside a loop — consider io.CopyBuffer
	walkLoopBodies(file, func(body *ast.BlockStmt) {
		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			id, ok := call.Fun.(*ast.Ident)
			if !ok || id.Name != "copy" {
				return true
			}
			f, line, col := nodePos(fset, call)
			issues = append(issues, Issue{
				Checker:    c.Name(),
				File:       f,
				Line:       line,
				Column:     col,
				Severity:   SeverityInfo,
				Message:    "copy() inside a loop — consider reslicing (read-only) or io.CopyBuffer (reusable buf)",
				Rule:       "Zero-Copy Techniques — https://goperf.dev/01-common-patterns/zero-copy/",
				Suggestion: "Use io.CopyBuffer(dst, src, reusableBuf) to avoid repeated allocations",
			})
			return true
		})
	})

	// 3. io.Copy in loop (should use io.CopyBuffer)
	walkLoopBodies(file, func(body *ast.BlockStmt) {
		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			pkg, fn, ok := callName(call)
			if !ok || pkg != "io" || fn != "Copy" {
				return true
			}
			f, line, col := nodePos(fset, call)
			issues = append(issues, Issue{
				Checker:    c.Name(),
				File:       f,
				Line:       line,
				Column:     col,
				Severity:   SeverityInfo,
				Message:    "io.Copy() in a loop allocates a new 32KB buffer every call",
				Rule:       "Zero-Copy Techniques — https://goperf.dev/01-common-patterns/zero-copy/",
				Suggestion: "Use io.CopyBuffer(dst, src, buf) with a buffer from sync.Pool",
			})
			return true
		})
	})

	return issues
}
