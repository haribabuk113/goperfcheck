package checker

import (
	"fmt"
	"go/ast"
	"go/token"
)

// LazyInitChecker flags expensive initialization at package level or in init()
// that could be deferred until first use via sync.Once or sync.OnceValue.
//
// Rule: https://goperf.dev/01-common-patterns/lazy-init/
// - Eager init() slows startup and allocates resources even if unused
// - Use sync.Once, sync.OnceValue, or sync.OnceValues for lazy init
type LazyInitChecker struct{}

func (c *LazyInitChecker) Name() string { return "LazyInit" }

// expensiveInitCalls are functions typically called for one-time setup.
var expensiveInitCalls = map[string]map[string]bool{
	"sql":  {"Open": true},
	"gorm": {"Open": true},
	"redis": {"Dial": true, "NewClient": true, "NewClusterClient": true},
	"mongo": {"Connect": true},
	"grpc": {"Dial": true, "NewServer": true},
	"http": {"ListenAndServe": true, "ListenAndServeTLS": true},
	"net":  {"Listen": true, "Dial": true, "DialTCP": true},
	"os":   {"Open": true, "Create": true},
}

func (c *LazyInitChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	var issues []Issue

	// 1. Expensive calls inside init() functions
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "init" || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			pkg, name, ok := callName(call)
			if !ok {
				return true
			}
			if fns, hasPkg := expensiveInitCalls[pkg]; hasPkg && fns[name] {
				f, line, col := nodePos(fset, call)
				issues = append(issues, Issue{
					Checker:    c.Name(),
					File:       f,
					Line:       line,
					Column:     col,
					Severity:   SeverityWarning,
					Message:    fmt.Sprintf("%s.%s() in init() — eager initialization slows startup even if unused", pkg, name),
					Rule:       "Lazy Initialization — https://goperf.dev/01-common-patterns/lazy-init/",
					Suggestion: "Wrap in sync.OnceValue(func() T { ... }) and call the getter on first use",
				})
			}
			return true
		})
	}

	// 2. Expensive calls at package-level var declarations
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, val := range vs.Values {
				call, ok := val.(*ast.CallExpr)
				if !ok {
					continue
				}
				pkg, name, ok := callName(call)
				if !ok {
					continue
				}
				if fns, hasPkg := expensiveInitCalls[pkg]; hasPkg && fns[name] {
					f, line, col := nodePos(fset, call)
					issues = append(issues, Issue{
						Checker:    c.Name(),
						File:       f,
						Line:       line,
						Column:     col,
						Severity:   SeverityWarning,
						Message:    fmt.Sprintf("package-level %s.%s() — initializes a resource at startup even if never used", pkg, name),
						Rule:       "Lazy Initialization — https://goperf.dev/01-common-patterns/lazy-init/",
						Suggestion: "Use sync.OnceValue / sync.OnceValues to defer initialization until first use",
					})
				}
			}
		}
	}

	return issues
}
