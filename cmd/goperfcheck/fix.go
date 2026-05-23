package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"sort"

	"github.com/haribabuk113/goperfcheck/checker"
)

// applyFixes applies all auto-fixable issues in place, grouped by file.
// Returns the number of individual fixes applied and any I/O error.
func applyFixes(issues []checker.Issue) (int, error) {
	byFile := make(map[string][]checker.Issue)
	for _, iss := range issues {
		if iss.Fix != nil {
			byFile[iss.File] = append(byFile[iss.File], iss)
		}
	}
	total := 0
	for path, fileIssues := range byFile {
		n, err := fixFile(path, fileIssues)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fix error %s: %v\n", path, err)
			continue
		}
		total += n
	}
	return total, nil
}

// fixFile applies all fixable issues in a single file, writes the result,
// and returns the number of edits applied.
func fixFile(path string, issues []checker.Issue) (int, error) {
	// Refuse to follow symlinks: a symlink to a file outside the scan root
	// would cause -fix to overwrite an unrelated file.
	fi, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return 0, fmt.Errorf("refusing to fix symlink: %s", path)
	}

	src, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return 0, err
	}

	type edit struct {
		start int
		end   int
		text  string
	}

	var edits []edit
	for _, iss := range issues {
		if iss.Fix == nil {
			continue
		}
		switch iss.Fix.Kind {
		case "map_cap":
			if e, ok := mapCapEdit(fset, file, iss); ok {
				edits = append(edits, e)
			}
		case "slice_cap":
			if e, ok := sliceCapEdit(fset, file, src, iss); ok {
				edits = append(edits, e)
			}
		case "struct_reorder":
			if e, ok := structReorderEdit(fset, file, src, iss); ok {
				edits = append(edits, e)
			}
		}
	}
	if len(edits) == 0 {
		return 0, nil
	}

	// Sort descending by start so we apply end-to-start (preserves offsets).
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })

	// Remove edits that overlap with a later-in-file (already accepted) edit.
	var clean []edit
	prevStart := len(src) + 1
	for _, e := range edits {
		if e.end <= prevStart {
			clean = append(clean, e)
			prevStart = e.start
		}
	}

	result := make([]byte, len(src))
	copy(result, src)
	for _, e := range clean {
		result = append(result[:e.start], append([]byte(e.text), result[e.end:]...)...)
	}

	formatted, err := format.Source(result)
	if err != nil {
		return 0, fmt.Errorf("format after fix: %w", err)
	}
	if err := os.WriteFile(path, formatted, 0o644); err != nil {
		return 0, err
	}
	return len(clean), nil
}

// mapCapEdit builds an edit that inserts ", cap" before the closing paren of
// a make(map[K]V) call at the position reported in iss.
func mapCapEdit(fset *token.FileSet, file *ast.File, iss checker.Issue) (struct {
	start, end int
	text       string
}, bool) {
	type edit struct {
		start, end int
		text       string
	}
	var found *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok || id.Name != "make" {
			return true
		}
		if len(call.Args) != 1 {
			return true // already has capacity or is invalid
		}
		if _, isMap := call.Args[0].(*ast.MapType); !isMap {
			return true
		}
		pos := fset.Position(call.Pos())
		if pos.Line == iss.Line && pos.Column == iss.Column {
			found = call
		}
		return true
	})
	if found == nil {
		return edit{}, false
	}
	tf := fset.File(found.Rparen)
	offset := tf.Offset(found.Rparen)
	return edit{start: offset, end: offset, text: ", " + iss.Fix.Cap}, true
}

// sliceCapEdit builds an edit that replaces "var x []T" immediately before the
// enclosing loop with "x := make([]T, 0, cap)". Returns false when the pattern
// does not match (non-var declaration, multiple vars, type not a slice, etc.).
func sliceCapEdit(fset *token.FileSet, file *ast.File, src []byte, iss checker.Issue) (struct {
	start, end int
	text       string
}, bool) {
	type edit struct {
		start, end int
		text       string
	}

	ctx := findEnclosingLoop(fset, file, iss.Line)
	if ctx == nil || ctx.index == 0 {
		return edit{}, false
	}

	prev := ctx.block.List[ctx.index-1]
	decl, ok := prev.(*ast.DeclStmt)
	if !ok {
		return edit{}, false
	}
	gen, ok := decl.Decl.(*ast.GenDecl)
	if !ok || gen.Tok != token.VAR || len(gen.Specs) != 1 {
		return edit{}, false
	}
	spec, ok := gen.Specs[0].(*ast.ValueSpec)
	if !ok || len(spec.Names) != 1 {
		return edit{}, false
	}
	if spec.Names[0].Name != iss.Fix.VarName {
		return edit{}, false
	}
	arr, ok := spec.Type.(*ast.ArrayType)
	if !ok || arr.Len != nil { // must be a slice, not fixed-size array
		return edit{}, false
	}
	// Only handle nil or absent initializer to avoid rewriting non-trivial values.
	if len(spec.Values) > 0 {
		id, ok := spec.Values[0].(*ast.Ident)
		if !ok || id.Name != "nil" {
			return edit{}, false
		}
	}

	var typeBuf bytes.Buffer
	if err := format.Node(&typeBuf, fset, arr); err != nil {
		return edit{}, false
	}

	newText := iss.Fix.VarName + " := make(" + typeBuf.String() + ", 0, " + iss.Fix.Cap + ")"
	tf := fset.File(prev.Pos())
	return edit{
		start: tf.Offset(prev.Pos()),
		end:   tf.Offset(prev.End()),
		text:  newText,
	}, true
}

// loopContext records an enclosing for/range loop and its position in the
// parent block. The innermost loop is always returned.
type loopContext struct {
	block *ast.BlockStmt
	index int
}

// structReorderEdit returns an edit that rewrites the fields of the named struct
// in the order prescribed by iss.Fix.FieldOrder (largest size first). It works
// at the source-line level: each *ast.Field's source lines (including any doc
// comment before it and any trailing line comment) are extracted verbatim and
// reassembled in the new order, so tags, alignment whitespace, and comments are
// all preserved exactly.
func structReorderEdit(fset *token.FileSet, file *ast.File, src []byte, iss checker.Issue) (struct {
	start, end int
	text       string
}, bool) {
	type edit struct {
		start, end int
		text       string
	}

	// Locate the struct by name.
	var target *ast.StructType
	ast.Inspect(file, func(n ast.Node) bool {
		if target != nil {
			return false
		}
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		if ts.Name.Name != iss.Fix.StructName {
			return true
		}
		if st, ok := ts.Type.(*ast.StructType); ok {
			target = st
		}
		return false
	})
	if target == nil || target.Fields == nil || len(target.Fields.List) < 2 {
		return edit{}, false
	}

	fields := target.Fields.List
	order := iss.Fix.FieldOrder
	if len(order) != len(fields) {
		return edit{}, false
	}

	tf := fset.File(fields[0].Pos())

	// lineStartOffset returns the byte offset of the first character of line l (1-based).
	lineStartOffset := func(l int) int {
		return tf.Offset(tf.LineStart(l))
	}
	// lineEndOffset returns the byte offset just past the '\n' that ends line l.
	// For the last line of the file (no trailing newline), it returns tf.Size().
	lineEndOffset := func(l int) int {
		if l < tf.LineCount() {
			return tf.Offset(tf.LineStart(l + 1))
		}
		return tf.Size()
	}

	// Compute the full line range for each field: extend backwards to include any
	// doc-comment block, and forwards to include any trailing line comment.
	type lineRange struct{ first, last int }
	ranges := make([]lineRange, len(fields))
	for i, f := range fields {
		first := fset.Position(f.Pos()).Line
		if f.Doc != nil {
			first = fset.Position(f.Doc.Pos()).Line
		}
		last := fset.Position(f.End()).Line
		if f.Comment != nil {
			last = fset.Position(f.Comment.End()).Line
		}
		ranges[i] = lineRange{first, last}
	}

	// Extract the raw source bytes for every field (entire lines, newline included).
	fieldBytes := make([][]byte, len(fields))
	for i, r := range ranges {
		fieldBytes[i] = src[lineStartOffset(r.first):lineEndOffset(r.last)]
	}

	// Assemble the reordered field text.
	var buf bytes.Buffer
	for _, idx := range order {
		buf.Write(fieldBytes[idx])
	}

	editStart := lineStartOffset(ranges[0].first)
	editEnd := lineEndOffset(ranges[len(ranges)-1].last)

	return edit{start: editStart, end: editEnd, text: buf.String()}, true
}

// findEnclosingLoop returns the loopContext for the innermost for/range loop
// whose span contains line. Returns nil if no loop contains line.
func findEnclosingLoop(fset *token.FileSet, file *ast.File, line int) *loopContext {
	var result *loopContext
	ast.Inspect(file, func(n ast.Node) bool {
		block, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for i, stmt := range block.List {
			var body *ast.BlockStmt
			switch s := stmt.(type) {
			case *ast.ForStmt:
				body = s.Body
			case *ast.RangeStmt:
				body = s.Body
			}
			if body == nil {
				continue
			}
			startLine := fset.Position(stmt.Pos()).Line
			endLine := fset.Position(stmt.End()).Line
			if line >= startLine && line <= endLine {
				result = &loopContext{block: block, index: i}
				// Do not return false — continue so an inner loop can override.
			}
		}
		return true
	})
	return result
}
