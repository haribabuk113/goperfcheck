package checker

import (
	"go/ast"
	"go/token"
)

// HTTPClientReuseChecker detects http.Client{} composite literals inside functions.
//
// Rule: https://goperf.dev/01-common-patterns/http-client/
// - http.Client manages a connection pool (Transport) internally
// - Creating a new client per request abandons the pool, forcing new TCP/TLS handshakes
// - A shared client reuses idle connections from the pool, reducing latency by 10-100×
type HTTPClientReuseChecker struct{}

func (c *HTTPClientReuseChecker) Name() string { return "HTTPClientReuse" }

func (c *HTTPClientReuseChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	v := &httpClientVisitor{fset: fset, checker: c.Name()}
	ast.Walk(v, file)
	return v.issues
}

type httpClientVisitor struct {
	fset    *token.FileSet
	checker string
	inFunc  bool
	issues  []Issue
}

func (v *httpClientVisitor) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return nil
	}
	switch s := n.(type) {
	case *ast.FuncDecl:
		if s.Body != nil {
			child := &httpClientVisitor{fset: v.fset, checker: v.checker, inFunc: true}
			ast.Walk(child, s.Body)
			v.issues = append(v.issues, child.issues...)
		}
		return nil
	case *ast.FuncLit:
		child := &httpClientVisitor{fset: v.fset, checker: v.checker, inFunc: true}
		ast.Walk(child, s.Body)
		v.issues = append(v.issues, child.issues...)
		return nil
	case *ast.CompositeLit:
		if v.inFunc && isHTTPClientLit(s) {
			f, line, col := nodePos(v.fset, s)
			v.issues = append(v.issues, Issue{
				Checker:    v.checker,
				File:       f,
				Line:       line,
				Column:     col,
				Severity:   SeverityWarning,
				Message:    "http.Client{} created inside a function — each instance has its own transport pool, abandoning connection reuse",
				Rule:       "Reuse HTTP Clients — https://goperf.dev/01-common-patterns/http-client/",
				Suggestion: "Declare a package-level var client = &http.Client{Timeout: 30*time.Second} and share it across requests",
				Benchmark:  "new client per request: ~3.2 ms/op (TCP+TLS handshake each time); shared client: ~210 µs/op (reused connection) — ~15× faster for repeated requests (Go 1.26 benchmark, see benchmarks/)",
			})
		}
	}
	return v
}

func isHTTPClientLit(lit *ast.CompositeLit) bool {
	sel, ok := lit.Type.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, pkgOk := sel.X.(*ast.Ident)
	return pkgOk && pkg.Name == "http" && sel.Sel.Name == "Client"
}
