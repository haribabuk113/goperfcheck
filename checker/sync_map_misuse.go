// Rule: https://goperf.dev/01-common-patterns/sync-map/
package checker

import (
	"go/ast"
	"go/token"
)

// SyncMapMisuseChecker flags declarations of sync.Map that are likely to be
// slower than a plain map[K]V guarded by sync.RWMutex.
//
// sync.Map is only faster in two narrow scenarios:
//  1. Append-only / stable-key caches: entries are written once then read
//     many times by many goroutines. The lock-free read path amortises the
//     one-time Store cost.
//  2. Disjoint per-goroutine key sets: each goroutine reads and writes its
//     own non-overlapping shard, so there is no meaningful RWMutex contention
//     to avoid.
//
// In all other patterns — growing registries, session maps, counters, or any
// workload where Store is called repeatedly on the same or new keys —
// sync.Map is slower than map+RWMutex and allocates 3 objects per Store
// because every key and value is boxed into interface{}.
//
// Patterns detected (all produce WARN):
//   - sync.Map or *sync.Map as a struct field
//   - var m sync.Map / var m *sync.Map (package-level or local)
//   - m := sync.Map{} (short variable declaration)
type SyncMapMisuseChecker struct{}

func (SyncMapMisuseChecker) Name() string { return "SyncMapMisuse" }

const (
	syncMapMsg = "sync.Map is only faster than map+sync.RWMutex for " +
		"append-only caches or per-goroutine disjoint key sets; " +
		"for growing maps or mixed read/write workloads it is slower " +
		"and boxes every key/value as interface{}"

	syncMapSuggestion = "use map[K]V + sync.RWMutex; keep sync.Map only when " +
		"keys are written once then read many times, or each goroutine " +
		"exclusively owns its key range"

	syncMapRule      = "sync.Map Misuse — https://goperf.dev/01-common-patterns/sync-map/"
	syncMapBenchmark = "sequential store+load: sync.Map 662 ns/op, 3 allocs/op " +
		"vs map+RWMutex 308 ns/op, 0 allocs/op (2× slower, interface{} boxing per Store)"
)

func (SyncMapMisuseChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	var issues []Issue

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {

		case *ast.StructType:
			if node.Fields == nil {
				return true
			}
			for _, field := range node.Fields.List {
				if isSyncMapType(field.Type) {
					f, l, c := nodePos(fset, field)
					issues = append(issues, Issue{
						Checker:    "SyncMapMisuse",
						File:       f,
						Line:       l,
						Column:     c,
						Severity:   SeverityWarning,
						Message:    "sync.Map as a struct field — " + syncMapMsg,
						Rule:       syncMapRule,
						Suggestion: syncMapSuggestion,
						Benchmark:  syncMapBenchmark,
					})
				}
			}

		case *ast.ValueSpec:
			// var m sync.Map  or  var m *sync.Map
			if isSyncMapType(node.Type) {
				f, l, c := nodePos(fset, node)
				issues = append(issues, Issue{
					Checker:    "SyncMapMisuse",
					File:       f,
					Line:       l,
					Column:     c,
					Severity:   SeverityWarning,
					Message:    "sync.Map variable — " + syncMapMsg,
					Rule:       syncMapRule,
					Suggestion: syncMapSuggestion,
					Benchmark:  syncMapBenchmark,
				})
				return true // type annotation already caught; skip value check
			}
			// var m = sync.Map{}
			for _, val := range node.Values {
				if cl, ok := val.(*ast.CompositeLit); ok && isSyncMapType(cl.Type) {
					f, l, c := nodePos(fset, cl)
					issues = append(issues, Issue{
						Checker:    "SyncMapMisuse",
						File:       f,
						Line:       l,
						Column:     c,
						Severity:   SeverityWarning,
						Message:    "sync.Map variable — " + syncMapMsg,
						Rule:       syncMapRule,
						Suggestion: syncMapSuggestion,
						Benchmark:  syncMapBenchmark,
					})
				}
			}

		case *ast.AssignStmt:
			if node.Tok != token.DEFINE {
				return true
			}
			// m := sync.Map{}
			for _, rhs := range node.Rhs {
				cl, ok := rhs.(*ast.CompositeLit)
				if !ok || !isSyncMapType(cl.Type) {
					continue
				}
				f, l, c := nodePos(fset, cl)
				issues = append(issues, Issue{
					Checker:    "SyncMapMisuse",
					File:       f,
					Line:       l,
					Column:     c,
					Severity:   SeverityWarning,
					Message:    "sync.Map variable — " + syncMapMsg,
					Rule:       syncMapRule,
					Suggestion: syncMapSuggestion,
					Benchmark:  syncMapBenchmark,
				})
			}
		}
		return true
	})

	return issues
}

// isSyncMapType reports whether expr resolves to sync.Map or *sync.Map.
func isSyncMapType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		id, ok := t.X.(*ast.Ident)
		return ok && id.Name == "sync" && t.Sel.Name == "Map"
	case *ast.StarExpr:
		return isSyncMapType(t.X)
	}
	return false
}
