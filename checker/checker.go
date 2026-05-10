// Package checker provides AST-based Go performance rule checkers
// derived from https://goperf.dev.
package checker

import (
	"go/ast"
	"go/token"
)

// Severity of the detected performance issue.
type Severity string

const (
	SeverityError   Severity = "ERROR"
	SeverityWarning Severity = "WARN"
	SeverityInfo    Severity = "INFO"
)

// FixHint carries machine-readable rewrite information for the -fix flag.
// Only a subset of issues are fixable; the rest leave Fix nil.
type FixHint struct {
	Kind    string `json:"kind"`          // "map_cap" | "slice_cap"
	VarName string `json:"var,omitempty"` // slice variable to update (slice_cap only)
	Cap     string `json:"cap"`           // capacity expression, e.g. "len(items)"
}

// Issue represents one performance problem found in source code.
type Issue struct {
	Checker    string   `json:"checker"`
	File       string   `json:"file"`
	Line       int      `json:"line"`
	Column     int      `json:"column"`
	Severity   Severity `json:"severity"`
	Message    string   `json:"message"`
	Rule       string   `json:"rule,omitempty"`
	Suggestion string   `json:"suggestion,omitempty"`
	Benchmark  string   `json:"benchmark,omitempty"`
	Fix        *FixHint `json:"fix,omitempty"`
	// VersionNote is set by ApplyGoVersion when the advice changes or may not
	// apply for the detected Go version. Empty for most issues.
	VersionNote string `json:"version_note,omitempty"`
}

// Checker is implemented by every performance rule.
type Checker interface {
	Name() string
	Check(fset *token.FileSet, file *ast.File) []Issue
}

// nodePos extracts (filename, line, column) from any AST node.
func nodePos(fset *token.FileSet, node ast.Node) (file string, line, col int) {
	p := fset.Position(node.Pos())
	return p.Filename, p.Line, p.Column
}

// callName returns the (package, funcName) pair for a call expression,
// e.g. "sync" + "NewMutex" for sync.NewMutex().
// For plain ident calls (append, make, new) package is "".
func callName(call *ast.CallExpr) (pkg, fn string, ok bool) {
	switch f := call.Fun.(type) {
	case *ast.Ident:
		return "", f.Name, true
	case *ast.SelectorExpr:
		if id, isID := f.X.(*ast.Ident); isID {
			return id.Name, f.Sel.Name, true
		}
	}
	return "", "", false
}

// walkLoopBodies calls fn once for every for/range loop body reachable
// from root (depth-first, including nested loops).
func walkLoopBodies(root ast.Node, fn func(body *ast.BlockStmt)) {
	ast.Inspect(root, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.ForStmt:
			if s.Body != nil {
				fn(s.Body)
			}
		case *ast.RangeStmt:
			if s.Body != nil {
				fn(s.Body)
			}
		}
		return true
	})
}

// dedupeIssues removes issues with identical (file, line, col, checker, message).
func DedupeIssues(issues []Issue) []Issue {
	seen := make(map[string]struct{}, len(issues))
	out := issues[:0:0]
	for _, iss := range issues {
		key := iss.File + ":" + string(rune(iss.Line)) + ":" + string(rune(iss.Column)) + ":" + iss.Checker + ":" + iss.Message
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			out = append(out, iss)
		}
	}
	return out
}
