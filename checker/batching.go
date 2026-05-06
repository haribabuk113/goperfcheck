package checker

import (
	"go/ast"
	"go/token"
)

// BatchingChecker detects individual I/O operations inside loops that would
// benefit from batching.
//
// Rule: https://goperf.dev/01-common-patterns/batching-ops/
// - Individual DB/Redis/HTTP calls in loops incur overhead per operation
// - Batching 1000 items in one call is vastly faster than 1000 individual calls
// - File I/O batching shows 12x speedup; crypto shows ~2x
type BatchingChecker struct{}

func (c *BatchingChecker) Name() string { return "Batching" }

// batchCandidates maps receiver variable names to method names that benefit from batching
var batchCandidates = map[string]map[string]bool{
	// Database / ORM
	"db":      {"Exec": true, "Query": true, "QueryRow": true, "Insert": true, "Create": true, "Save": true},
	"tx":      {"Exec": true, "Query": true},
	"gorm":    {"Create": true, "Save": true, "Delete": true},
	"sqlDB":   {"Exec": true, "Query": true},
	"session": {"Query": true, "Exec": true},

	// Redis / cache
	"rdb":    {"Set": true, "Get": true, "HSet": true, "HGet": true, "Del": true, "ZAdd": true},
	"client": {"Set": true, "Get": true, "HSet": true, "Do": true, "Post": true},

	// HTTP
	"httpClient": {"Do": true, "Get": true, "Post": true},

	// Message producers
	"producer": {"SendMessage": true, "SendMessages": true},
	"kafka":    {"Produce": true},
}

func (c *BatchingChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	var issues []Issue

	walkLoopBodies(file, func(body *ast.BlockStmt) {
		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, isSel := call.Fun.(*ast.SelectorExpr)
			if !isSel {
				return true
			}
			methodName := sel.Sel.Name
			receiverName := ""
			if id, ok := sel.X.(*ast.Ident); ok {
				receiverName = id.Name
			}

			// Match by receiver variable name heuristic
			if methods, ok := batchCandidates[receiverName]; ok && methods[methodName] {
				f, line, col := nodePos(fset, call)
				issues = append(issues, Issue{
					Checker:    c.Name(),
					File:       f,
					Line:       line,
					Column:     col,
					Severity:   SeverityWarning,
					Message:    receiverName + "." + methodName + "() called inside a loop — each call is a separate operation with overhead",
					Rule:       "Batching Operations — https://goperf.dev/01-common-patterns/batching-ops/",
					Suggestion: "Collect items in a slice; execute one bulk operation outside the loop (batch insert, pipeline, bulk post)",
				})
			}
			return true
		})
	})

	return issues
}
