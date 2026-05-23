package checker

import (
	"fmt"
	"go/ast"
	"go/token"
	"sort"
	"strings"
)

// StructAlignChecker flags struct field orderings that waste memory through padding.
//
// Rule: https://goperf.dev/01-common-patterns/fields-alignment/
// Best practice: order fields from largest to smallest to minimize alignment padding.
// Example: 80MB wasted per 10M instances when small and large fields are interleaved.
type StructAlignChecker struct{}

func (c *StructAlignChecker) Name() string { return "StructAlign" }

// approxSize returns an approximate size in bytes for a field type.
func approxSize(expr ast.Expr) int {
	switch t := expr.(type) {
	case *ast.Ident:
		switch t.Name {
		case "bool", "byte", "int8", "uint8":
			return 1
		case "int16", "uint16":
			return 2
		case "int32", "uint32", "float32", "rune":
			return 4
		case "int", "uint", "int64", "uint64", "float64", "complex64", "uintptr":
			return 8
		case "complex128":
			return 16
		case "string":
			return 16
		default:
			return 8 // assume struct/type = pointer sized
		}
	case *ast.StarExpr:
		return 8 // pointer
	case *ast.ArrayType:
		if t.Len == nil {
			return 24 // slice: ptr + len + cap
		}
		return 8 // fixed array
	case *ast.MapType, *ast.ChanType, *ast.FuncType:
		return 8
	case *ast.InterfaceType:
		return 16
	case *ast.SelectorExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			switch id.Name + "." + t.Sel.Name {
			case "sync.Mutex":
				return 8
			case "sync.RWMutex":
				return 24
			case "sync.Once", "sync.WaitGroup":
				return 12
			case "atomic.Value":
				return 16
			case "time.Time":
				return 24
			case "time.Duration":
				return 8
			}
		}
		return 8
	}
	return 8
}

func (c *StructAlignChecker) Check(fset *token.FileSet, file *ast.File) []Issue {
	var issues []Issue

	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok || st.Fields == nil || len(st.Fields.List) < 3 {
			return true
		}

		type fld struct {
			name string
			size int
			pos  token.Pos
		}

		var fields []fld
		for _, field := range st.Fields.List {
			sz := approxSize(field.Type)
			name := "_"
			if len(field.Names) > 0 {
				ns := make([]string, len(field.Names))
				for i, nm := range field.Names {
					ns[i] = nm.Name
				}
				name = strings.Join(ns, "/")
			}
			fields = append(fields, fld{name: name, size: sz, pos: field.Pos()})
		}

		// Detect "bad" transitions: small field immediately before significantly larger one.
		// This indicates potential padding waste.
		exported := len(ts.Name.Name) > 0 && ts.Name.Name[0] >= 'A' && ts.Name.Name[0] <= 'Z'
		for i := 0; i < len(fields)-1; i++ {
			if fields[i].size < fields[i+1].size && fields[i].size <= 4 && fields[i+1].size >= 8 {
				p := fset.Position(fields[i].pos)

				suggestion := "Reorder fields largest → smallest: int64/pointers first, then int32, int16, bool/byte last"
				if exported {
					suggestion = fmt.Sprintf(
						"Exported type — audit callers for positional struct literals (%s{v1, v2, …}) before reordering; "+
							"reordering exported fields is a breaking API change for any caller that omits field names. "+
							"If all call sites use named fields: reorder largest → smallest, int64/pointers first, then int32, int16, bool/byte last",
						ts.Name.Name,
					)
				}

				// Compute the optimal field ordering (largest → smallest, stable).
				// Indices map into st.Fields.List so fix.go can locate the exact AST nodes.
				type idxSize struct{ idx, size int }
				sortable := make([]idxSize, len(fields))
				for j, f := range fields {
					sortable[j] = idxSize{idx: j, size: f.size}
				}
				sort.SliceStable(sortable, func(a, b int) bool {
					return sortable[a].size > sortable[b].size
				})
				order := make([]int, len(sortable))
				for j, s := range sortable {
					order[j] = s.idx
				}

				issues = append(issues, Issue{
					Checker:  c.Name(),
					File:     p.Filename,
					Line:     p.Line,
					Column:   p.Column,
					Severity: SeverityWarning,
					Message: fmt.Sprintf(
						"struct %q: field %q (~%dB) before %q (~%dB) — misalignment causes padding waste",
						ts.Name.Name, fields[i].name, fields[i].size,
						fields[i+1].name, fields[i+1].size,
					),
					Rule:       "Struct Field Alignment — https://goperf.dev/01-common-patterns/fields-alignment/",
					Suggestion: suggestion,
					Benchmark:  "32B → 24B per instance (25% less memory) for a {bool,int64,bool,int64} struct — GC work scales with live heap size (Go 1.26 benchmark, see benchmarks/)",
					Fix: &FixHint{
						Kind:       "struct_reorder",
						StructName: ts.Name.Name,
						FieldOrder: order,
					},
				})
				break // one issue per struct
			}
		}
		return true
	})

	return issues
}
