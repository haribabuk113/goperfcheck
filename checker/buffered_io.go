package checker

import (
	"go/ast"
	"go/token"
)

// BufferedIOChecker detects unbuffered file writes and missing Flush() calls.
//
// Rule: https://goperf.dev/01-common-patterns/buffered-io/
// - Unbuffered writes in loops cause many syscalls (10,000+ syscalls vs handful with buffering)
// - bufio.Writer does NOT auto-flush on close — forgotten Flush() causes data loss
// - Buffering can reduce syscalls by ~12x in realistic scenarios
type BufferedIOChecker struct{}

func (c *BufferedIOChecker) Name() string { return "BufferedIO" }

func (c *BufferedIOChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	var issues []Issue

	// 1. Direct .Write / .WriteString on file-like receivers inside loops
	directWriteMethods := map[string]bool{
		"Write":       true,
		"WriteString": true,
		"WriteByte":   true,
		"WriteRune":   true,
	}

	walkLoopBodies(file, func(body *ast.BlockStmt) {
		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if !directWriteMethods[sel.Sel.Name] {
				return true
			}
			// Check if receiver is a simple identifier (conservative check)
			if _, ok := sel.X.(*ast.Ident); ok {
				f, line, col := nodePos(fset, call)
				issues = append(issues, Issue{
					Checker:    c.Name(),
					File:       f,
					Line:       line,
					Column:     col,
					Severity:   SeverityWarning,
					Message:    "." + sel.Sel.Name + "() in a loop — each call may trigger a syscall",
					Rule:       "Efficient Buffering — https://goperf.dev/01-common-patterns/buffered-io/",
					Suggestion: "Wrap the writer with bufio.NewWriter(w); call Flush() after the loop (or use defer)",
				})
			}
			return true
		})
	})

	// 2. bufio.NewWriter/NewReader created but Flush() never called in the same function
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}

		type bufferVar struct {
			name string
			pos  token.Pos
		}
		var buffers []bufferVar
		var hasFlushed bool

		ast.Inspect(fn.Body, func(inner ast.Node) bool {
			// Look for bufio.NewWriter/NewReader assignments
			assign, ok := inner.(*ast.AssignStmt)
			if ok {
				for i, rhs := range assign.Rhs {
					call, ok := rhs.(*ast.CallExpr)
					if !ok {
						continue
					}
					pkg, name, ok := callName(call)
					if !ok || pkg != "bufio" || name != "NewWriter" && name != "NewReader" {
						continue
					}
					if i < len(assign.Lhs) {
						if id, ok := assign.Lhs[i].(*ast.Ident); ok {
							buffers = append(buffers, bufferVar{name: id.Name, pos: inner.Pos()})
						}
					}
				}
			}
			// Check for Flush calls
			if call, ok := inner.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if sel.Sel.Name == "Flush" {
						hasFlushed = true
					}
				}
			}
			return true
		})

		if len(buffers) > 0 && !hasFlushed {
			for _, buf := range buffers {
				p := fset.Position(buf.pos)
				issues = append(issues, Issue{
					Checker:    c.Name(),
					File:       p.Filename,
					Line:       p.Line,
					Column:     p.Column,
					Severity:   SeverityError,
					Message:    "bufio.NewWriter(\"" + buf.name + "\") created but Flush() never called — buffered data will be lost",
					Rule:       "Efficient Buffering — https://goperf.dev/01-common-patterns/buffered-io/",
					Suggestion: "Add defer " + buf.name + ".Flush() immediately after creating the bufio.Writer",
				})
			}
		}
		return true
	})

	return issues
}
