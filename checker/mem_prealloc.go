package checker

import (
	"fmt"
	"go/ast"
	"go/token"
)

// defaultCapHint is suggested when the checker cannot infer a size from code context.
// A small non-zero hint is always preferable to none: it prevents the first few
// doublings (0→1→2→4→8 for slices, or the first rehash for maps) at the cost of
// at most 8×sizeof(element) bytes — memory that would be allocated anyway once
// the data structure grows past that point.
const defaultCapHint = "8"

// MemPreallocChecker detects slice/map allocations without capacity hints.
//
// Rule: https://goperf.dev/01-common-patterns/mem-prealloc/
//   - append() in a loop without a preallocated capacity causes repeated
//     reallocations (~4x slower, ~19x more allocations in benchmarks).
//   - make(map[K]V) without a size hint causes rehashing as the map grows.
//
// # Size-hint inference
//
// The checker tries to derive the best capacity hint from the surrounding code:
//
//  1. append() in a range loop  →  suggest len(<rangeExpr>)
//  2. append() in a C-style for loop  →  extract the upper bound (n for i < n,
//     n+1 for i <= n)
//  3. make(map) assigned to a variable, followed in the same block by a range
//     loop that assigns into that map  →  suggest len(<rangeExpr>)
//  4. Any other case  →  suggest the conservative default of 8
//
// Why a default of 8 is better than no hint:
//
// For slices, omitting a capacity causes the runtime to allocate and copy the
// backing array ~log2(finalLen) times.  Preallocating 8 slots eliminates the
// first three doublings (cap 0→1→2→4→8) which are the most expensive relative
// to the work done.  For maps, Go's runtime rehashes at a load-factor of ~6.5/8,
// so make(map[K]V, 8) avoids the first rehash entirely.  In both cases the
// downside — a tiny allocation that would have been made anyway — is negligible
// compared with the saved reallocations.
type MemPreallocChecker struct{}

func (c *MemPreallocChecker) Name() string { return "MemPrealloc" }

func (c *MemPreallocChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	var issues []Issue
	issues = append(issues, c.checkAppendInLoops(fset, file)...)
	issues = append(issues, c.checkMapMake(fset, file)...)
	return issues
}

// checkAppendInLoops walks every for/range loop in root and reports append()
// calls found directly inside that loop body (not in nested loops).
// The capacity hint is derived from the loop structure itself.
func (c *MemPreallocChecker) checkAppendInLoops(fset *token.FileSet, root ast.Node) []Issue {
	var issues []Issue
	ast.Inspect(root, func(n ast.Node) bool {
		switch loop := n.(type) {
		case *ast.RangeStmt:
			hint := rangeExprHint(loop.X)
			issues = append(issues, c.appendIssuesInBody(fset, loop.Body, hint)...)
		case *ast.ForStmt:
			hint := forLoopCapHint(loop)
			issues = append(issues, c.appendIssuesInBody(fset, loop.Body, hint)...)
		}
		return true
	})
	return issues
}

// appendIssuesInBody finds append() calls directly in body, stopping at nested
// loops so each append is attributed to its innermost enclosing loop.
func (c *MemPreallocChecker) appendIssuesInBody(fset *token.FileSet, body *ast.BlockStmt, hint string) []Issue {
	var issues []Issue
	if body == nil {
		return nil
	}
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		// Stop at nested loops — the outer ast.Inspect call handles them
		// with their own (more specific) hint.
		switch n.(type) {
		case *ast.RangeStmt, *ast.ForStmt:
			return false
		}
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
			Checker:  c.Name(),
			File:     f,
			Line:     line,
			Column:   col,
			Severity: SeverityWarning,
			Message:  "append() inside a loop — repeated reallocations occur when the backing array runs out of capacity",
			Rule:     "Memory Preallocation — https://goperf.dev/01-common-patterns/mem-prealloc/",
			Suggestion: fmt.Sprintf(
				"Before the loop use make([]T, 0, %s); even a small hint avoids the costliest early reallocations",
				hint,
			),
		})
		return true
	})
	return issues
}

// checkMapMake finds make(map[K]V) calls with no size argument.
// For assignment contexts it tries to infer the hint from a subsequent range
// loop in the same block; all other cases fall back to defaultCapHint.
func (c *MemPreallocChecker) checkMapMake(fset *token.FileSet, root ast.Node) []Issue {
	var issues []Issue
	reported := make(map[token.Pos]bool)

	// Pass 1: block-level analysis for assignment contexts.
	// Walking every BlockStmt lets us inspect sequential statements and find
	// a range loop that follows the make(map) in the same scope.
	ast.Inspect(root, func(n ast.Node) bool {
		block, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for i, stmt := range block.List {
			assign, ok := stmt.(*ast.AssignStmt)
			if !ok {
				continue
			}
			for j, rhs := range assign.Rhs {
				call := unmakeMapNoHint(rhs)
				if call == nil {
					continue
				}
				reported[call.Pos()] = true

				hint := ""
				if j < len(assign.Lhs) {
					if id, ok := assign.Lhs[j].(*ast.Ident); ok && id.Name != "_" {
						hint = c.rangeHintForMapVar(block.List, i+1, id.Name)
					}
				}
				if hint == "" {
					hint = defaultCapHint
				}
				issues = append(issues, c.mapIssue(fset, call, hint))
			}
		}
		return true
	})

	// Pass 2: any remaining make(map) not in a direct assignment (e.g. passed
	// as a function argument).  No context to infer from — use the default.
	ast.Inspect(root, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if unmakeMapNoHint(call) == nil || reported[call.Pos()] {
			return true
		}
		issues = append(issues, c.mapIssue(fset, call, defaultCapHint))
		return true
	})

	return issues
}

// rangeHintForMapVar scans statements after startIdx looking for a range loop
// whose body assigns into varName[key].  Returns "len(rangeExpr)" when found.
func (c *MemPreallocChecker) rangeHintForMapVar(stmts []ast.Stmt, startIdx int, varName string) string {
	for i := startIdx; i < len(stmts); i++ {
		rng, ok := stmts[i].(*ast.RangeStmt)
		if !ok {
			continue
		}
		if c.bodyWritesToMapVar(rng.Body, varName) {
			if s := exprStr(rng.X); s != "" {
				return "len(" + s + ")"
			}
		}
	}
	return ""
}

// bodyWritesToMapVar reports whether body contains an index-assignment (varName[key] = ...).
func (c *MemPreallocChecker) bodyWritesToMapVar(body *ast.BlockStmt, varName string) bool {
	if body == nil {
		return false
	}
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			if idx, ok := lhs.(*ast.IndexExpr); ok {
				if id, ok := idx.X.(*ast.Ident); ok && id.Name == varName {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

func (c *MemPreallocChecker) mapIssue(fset *token.FileSet, call *ast.CallExpr, hint string) Issue {
	f, line, col := nodePos(fset, call)
	return Issue{
		Checker:  c.Name(),
		File:     f,
		Line:     line,
		Column:   col,
		Severity: SeverityInfo,
		Message:  fmt.Sprintf("make(map) on line %d has no capacity hint — map will rehash every time it doubles in size", line),
		Rule:     "Memory Preallocation — https://goperf.dev/01-common-patterns/mem-prealloc/",
		Suggestion: fmt.Sprintf(
			"Use make(map[K]V, %s) to avoid rehashing; even a small hint prevents the first rehash",
			hint,
		),
	}
}

// unmakeMapNoHint returns call if expr is make(map[K]V) with no size argument, else nil.
func unmakeMapNoHint(expr ast.Expr) *ast.CallExpr {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok || id.Name != "make" || len(call.Args) == 0 {
		return nil
	}
	if _, isMap := call.Args[0].(*ast.MapType); isMap && len(call.Args) == 1 {
		return call
	}
	return nil
}

// exprStr returns a concise string for common AST expression forms.
// Returns "" for expressions it cannot represent simply.
func exprStr(e ast.Expr) string {
	if e == nil {
		return ""
	}
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		if x := exprStr(v.X); x != "" {
			return x + "." + v.Sel.Name
		}
		return v.Sel.Name
	case *ast.BasicLit:
		return v.Value
	case *ast.ParenExpr:
		return exprStr(v.X)
	case *ast.CallExpr:
		if fn := exprStr(v.Fun); fn != "" {
			return fn + "(...)"
		}
	}
	return ""
}

// rangeExprHint returns the capacity hint for a range loop over expr.
func rangeExprHint(expr ast.Expr) string {
	s := exprStr(expr)
	if s == "" {
		return defaultCapHint
	}
	return "len(" + s + ")"
}

// forLoopCapHint extracts a capacity hint from a C-style for loop condition.
// Handles: i < n  →  n;  i <= n  →  n+1.
// Returns defaultCapHint when the pattern is not recognised.
func forLoopCapHint(loop *ast.ForStmt) string {
	if loop.Cond == nil {
		return defaultCapHint
	}
	bin, ok := loop.Cond.(*ast.BinaryExpr)
	if !ok {
		return defaultCapHint
	}
	switch bin.Op {
	case token.LSS: // i < n  →  capacity n
		if s := exprStr(bin.Y); s != "" {
			return s
		}
	case token.LEQ: // i <= n  →  capacity n+1
		if s := exprStr(bin.Y); s != "" {
			return s + "+1"
		}
	}
	return defaultCapHint
}
