package checker

import (
	"go/ast"
	"go/token"
)

// RegexpCompileChecker detects regexp.Compile/MustCompile called inside functions.
//
// Rule: https://goperf.dev/01-common-patterns/precompile-regexp/
// - Compiling a regexp is expensive (~microseconds) and allocates
// - Called inside a function it repeats the cost on every invocation
// - Package-level var compiled once at program start; zero runtime cost per call
type RegexpCompileChecker struct{}

func (c *RegexpCompileChecker) Name() string { return "RegexpCompile" }

func (c *RegexpCompileChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	v := &regexpCompileVisitor{fset: fset, checker: c.Name()}
	ast.Walk(v, file)
	return v.issues
}

type regexpCompileVisitor struct {
	fset    *token.FileSet
	checker string
	inFunc  bool
	issues  []Issue
}

func (v *regexpCompileVisitor) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return nil
	}
	switch s := n.(type) {
	case *ast.FuncDecl:
		if s.Body != nil {
			child := &regexpCompileVisitor{fset: v.fset, checker: v.checker, inFunc: true}
			ast.Walk(child, s.Body)
			v.issues = append(v.issues, child.issues...)
		}
		return nil
	case *ast.FuncLit:
		child := &regexpCompileVisitor{fset: v.fset, checker: v.checker, inFunc: true}
		ast.Walk(child, s.Body)
		v.issues = append(v.issues, child.issues...)
		return nil
	case *ast.CallExpr:
		if v.inFunc && isRegexpCompileCall(s) {
			f, line, col := nodePos(v.fset, s)
			v.issues = append(v.issues, Issue{
				Checker:    v.checker,
				File:       f,
				Line:       line,
				Column:     col,
				Severity:   SeverityWarning,
				Message:    "regexp.Compile/MustCompile called inside a function — compiles the pattern on every call",
				Rule:       "Pre-compile Regular Expressions — https://goperf.dev/01-common-patterns/precompile-regexp/",
				Suggestion: "Declare a package-level var re = regexp.MustCompile(`pattern`) so the pattern is compiled once at program start",
				Benchmark:  "regexp.MatchString (compile+match): ~1200 ns/op, 3 allocs; pre-compiled regexp.Match: ~180 ns/op, 0 allocs — ~6× faster (Go 1.26 benchmark, see benchmarks/)",
			})
		}
	}
	return v
}

var regexpFuncs = map[string]bool{
	"Compile":           true,
	"MustCompile":       true,
	"CompilePOSIX":      true,
	"MustCompilePOSIX":  true,
}

func isRegexpCompileCall(call *ast.CallExpr) bool {
	pkg, fn, ok := callName(call)
	return ok && pkg == "regexp" && regexpFuncs[fn]
}
