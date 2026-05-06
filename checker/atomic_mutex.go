package checker

import (
	"fmt"
	"go/ast"
	"go/token"
)

// AtomicMutexChecker flags sync.Mutex use for simple scalar fields where atomics
// would be significantly faster.
//
// Rule: https://goperf.dev/01-common-patterns/atomic-ops/
// - Atomic operations ~27% faster than mutex under contention
// - Atomics suitable for counters, flags, boolean states
// - Mutexes needed for complex transactional state
type AtomicMutexChecker struct{}

func (c *AtomicMutexChecker) Name() string { return "AtomicMutex" }

func isMutexType(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return id.Name == "sync" && (sel.Sel.Name == "Mutex" || sel.Sel.Name == "RWMutex")
}

func isAtomicSuitable(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	switch id.Name {
	case "int", "int32", "int64", "uint", "uint32", "uint64", "bool", "uintptr":
		return true
	}
	return false
}

func (c *AtomicMutexChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	var issues []Issue

	// Detect structs with sync.Mutex protecting only atomic-suitable scalar fields
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok || st.Fields == nil {
			return true
		}

		hasMutex := false
		atomicCandidates := 0
		nonAtomicFields := 0

		for _, field := range st.Fields.List {
			if isMutexType(field.Type) {
				hasMutex = true
				continue
			}
			if isAtomicSuitable(field.Type) {
				atomicCandidates++
			} else {
				nonAtomicFields++
			}
		}

		if hasMutex && atomicCandidates > 0 && nonAtomicFields == 0 {
			f, line, col := nodePos(fset, ts)
			issues = append(issues, Issue{
				Checker:  c.Name(),
				File:     f,
				Line:     line,
				Column:   col,
				Severity: SeverityInfo,
				Message: fmt.Sprintf(
					"struct %q uses sync.Mutex to protect only atomic-compatible scalar fields — consider sync/atomic.Int64 / Bool / Uint64",
					ts.Name.Name,
				),
				Rule:       "Atomic Operations — https://goperf.dev/01-common-patterns/atomic-ops/",
				Suggestion: "Replace sync.Mutex with atomic.Int64, atomic.Bool, or atomic.Uint64 (27% faster under contention)",
			})
		}
		return true
	})

	// Detect Lock/incr/Unlock patterns that could be atomic.Add
	ast.Inspect(file, func(n ast.Node) bool {
		block, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for i := 0; i+2 < len(block.List); i++ {
			if c.isLockCall(block.List[i]) && c.isUnlockCall(block.List[i+2]) {
				if c.isSimpleIncDec(block.List[i+1]) {
					p := fset.Position(block.List[i].Pos())
					issues = append(issues, Issue{
						Checker:    c.Name(),
						File:       p.Filename,
						Line:       p.Line,
						Column:     p.Column,
						Severity:   SeverityWarning,
						Message:    "mutex protecting a single increment/decrement — use atomic.AddInt64 / AddInt32 instead",
						Rule:       "Atomic Operations — https://goperf.dev/01-common-patterns/atomic-ops/",
						Suggestion: "Replace with atomic.Add* (~27% faster) — no goroutine blocking",
					})
				}
			}
		}
		return true
	})

	return issues
}

func (c *AtomicMutexChecker) isLockCall(stmt ast.Stmt) bool {
	expr, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expr.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	_, fn, ok := callName(call)
	return ok && fn == "Lock"
}

func (c *AtomicMutexChecker) isUnlockCall(stmt ast.Stmt) bool {
	expr, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expr.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	_, fn, ok := callName(call)
	return ok && fn == "Unlock"
}

func (c *AtomicMutexChecker) isSimpleIncDec(stmt ast.Stmt) bool {
	switch s := stmt.(type) {
	case *ast.IncDecStmt:
		_ = s
		return true
	case *ast.AssignStmt:
		if s.Tok.String() == "+=" || s.Tok.String() == "-=" {
			return true
		}
	}
	return false
}
