package checker

import (
	"go/ast"
	"go/token"
)

// StringConcatLoopChecker detects string concatenation with + inside loops.
//
// Rule: https://goperf.dev/01-common-patterns/string-building/
// - Each `s += x` or `s = s + x` in a loop allocates a new string and copies the old bytes
// - O(n²) allocation growth for n iterations — strings.Builder is O(n)
// - strings.Builder.WriteString is allocation-free after the initial Grow
type StringConcatLoopChecker struct{}

func (c *StringConcatLoopChecker) Name() string { return "StringConcatLoop" }

func (c *StringConcatLoopChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	v := &stringConcatVisitor{fset: fset, checker: c.Name()}
	ast.Walk(v, file)
	return v.issues
}

type stringConcatVisitor struct {
	fset    *token.FileSet
	checker string
	inLoop  bool
	issues  []Issue
}

func (v *stringConcatVisitor) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return nil
	}
	switch s := n.(type) {
	case *ast.FuncLit:
		child := &stringConcatVisitor{fset: v.fset, checker: v.checker, inLoop: false}
		ast.Walk(child, s.Body)
		v.issues = append(v.issues, child.issues...)
		return nil
	case *ast.ForStmt:
		child := &stringConcatVisitor{fset: v.fset, checker: v.checker, inLoop: true}
		ast.Walk(child, s.Body)
		v.issues = append(v.issues, child.issues...)
		return nil
	case *ast.RangeStmt:
		child := &stringConcatVisitor{fset: v.fset, checker: v.checker, inLoop: true}
		ast.Walk(child, s.Body)
		v.issues = append(v.issues, child.issues...)
		return nil
	case *ast.AssignStmt:
		if v.inLoop && isStringConcatAssign(s) {
			f, line, col := nodePos(v.fset, s)
			v.issues = append(v.issues, Issue{
				Checker:    v.checker,
				File:       f,
				Line:       line,
				Column:     col,
				Severity:   SeverityWarning,
				Message:    "string concatenation with + in a loop causes O(n²) allocations — each iteration copies all previous bytes",
				Rule:       "String Building — https://goperf.dev/01-common-patterns/string-building/",
				Suggestion: "Use strings.Builder: declare before the loop, call b.WriteString(x) inside, return b.String() after",
				Benchmark:  "string += in loop (1000 iters): ~520 µs, 999 allocs; strings.Builder: ~5 µs, 1 alloc — ~100× faster (Go 1.26 benchmark, see benchmarks/)",
			})
		}
	}
	return v
}

// isStringConcatAssign returns true for `s += expr` (ADD_ASSIGN)
// or `s = s + expr` / `s = expr + s` where the LHS variable appears in the RHS addition.
func isStringConcatAssign(a *ast.AssignStmt) bool {
	if len(a.Lhs) != 1 || len(a.Rhs) != 1 {
		return false
	}
	lhsIdent, ok := a.Lhs[0].(*ast.Ident)
	if !ok {
		return false
	}

	// `s += expr`
	if a.Tok.String() == "+=" {
		return true
	}

	// `s = s + expr` or `s = expr + s`
	if a.Tok.String() != "=" {
		return false
	}
	bin, ok := a.Rhs[0].(*ast.BinaryExpr)
	if !ok || bin.Op.String() != "+" {
		return false
	}
	leftIdent, leftOk := bin.X.(*ast.Ident)
	rightIdent, rightOk := bin.Y.(*ast.Ident)
	return (leftOk && leftIdent.Name == lhsIdent.Name) ||
		(rightOk && rightIdent.Name == lhsIdent.Name)
}
