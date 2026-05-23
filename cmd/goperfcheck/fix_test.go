package main

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/haribabuk113/goperfcheck/checker"
)

// applyStructReorderFix is a test helper that runs the StructAlignChecker on
// src, finds the first fixable issue, applies structReorderEdit, and returns
// the rewritten source. Fails the test if no fixable issue is found.
func applyStructReorderFix(t *testing.T, src string) string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	c := &checker.StructAlignChecker{}
	issues := c.Check(fset, file)

	var fixable *checker.Issue
	for i := range issues {
		if issues[i].Fix != nil && issues[i].Fix.Kind == "struct_reorder" {
			fixable = &issues[i]
			break
		}
	}
	if fixable == nil {
		t.Fatal("no struct_reorder fix found in issues")
	}

	e, ok := structReorderEdit(fset, file, []byte(src), *fixable)
	if !ok {
		t.Fatal("structReorderEdit returned false")
	}

	b := []byte(src)
	result := append(b[:e.start], append([]byte(e.text), b[e.end:]...)...)
	return string(result)
}

func TestStructReorderEditBasic(t *testing.T) {
	src := `package p

type Bad struct {
	flag bool
	val  int64
	name string
}
`
	got := applyStructReorderFix(t, src)

	// val (8B) and name (16B) must come before flag (1B).
	valIdx := strings.Index(got, "val")
	nameIdx := strings.Index(got, "name")
	flagIdx := strings.Index(got, "flag")

	if flagIdx < nameIdx || flagIdx < valIdx {
		t.Errorf("flag should appear after val and name in reordered struct:\n%s", got)
	}
}

func TestStructReorderEditPreservesInlineComments(t *testing.T) {
	src := `package p

type Bad struct {
	flag bool   // whether active
	val  int64  // counter value
	name string // identifier
}
`
	got := applyStructReorderFix(t, src)

	// Each comment must still be on the same line as its field.
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "flag") && !strings.Contains(line, "whether active") {
			t.Errorf("inline comment lost from flag line: %q", line)
		}
		if strings.Contains(line, "val") && strings.Contains(line, "int64") && !strings.Contains(line, "counter value") {
			t.Errorf("inline comment lost from val line: %q", line)
		}
		if strings.Contains(line, "name") && strings.Contains(line, "string") && !strings.Contains(line, "identifier") {
			t.Errorf("inline comment lost from name line: %q", line)
		}
	}
}

func TestStructReorderEditPreservesStructTags(t *testing.T) {
	src := `package p

type Bad struct {
	Flag bool   ` + "`json:\"flag\"`" + `
	Val  int64  ` + "`json:\"val\"`" + `
	Name string ` + "`json:\"name\"`" + `
}
`
	got := applyStructReorderFix(t, src)

	// Each tag must still be on the same line as its field.
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "Flag") && !strings.Contains(line, `"flag"`) {
			t.Errorf("struct tag lost from Flag line: %q", line)
		}
		if strings.Contains(line, "Val") && strings.Contains(line, "int64") && !strings.Contains(line, `"val"`) {
			t.Errorf("struct tag lost from Val line: %q", line)
		}
		if strings.Contains(line, "Name") && strings.Contains(line, "string") && !strings.Contains(line, `"name"`) {
			t.Errorf("struct tag lost from Name line: %q", line)
		}
	}
}

func TestStructReorderEditPreservesDocComments(t *testing.T) {
	src := `package p

type Bad struct {
	// boolean flag field
	flag bool
	// 64-bit counter
	val int64
	// display name
	name string
}
`
	got := applyStructReorderFix(t, src)

	// Each doc comment must immediately precede its field after reordering.
	lines := strings.Split(got, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "val int64" && i > 0 {
			prev := strings.TrimSpace(lines[i-1])
			if prev != "// 64-bit counter" {
				t.Errorf("doc comment for val not adjacent after reorder; prev line = %q", prev)
			}
		}
		if strings.TrimSpace(line) == "flag bool" && i > 0 {
			prev := strings.TrimSpace(lines[i-1])
			if prev != "// boolean flag field" {
				t.Errorf("doc comment for flag not adjacent after reorder; prev line = %q", prev)
			}
		}
	}
}

func TestStructReorderEditMultipleStructs(t *testing.T) {
	// Only the struct with the fix hint should be reordered; the other is untouched.
	src := `package p

type Good struct {
	val  int64
	name string
	flag bool
}

type Bad struct {
	flag bool
	val  int64
	name string
}
`
	got := applyStructReorderFix(t, src)

	// Good struct must still start with val.
	goodBlock := got[strings.Index(got, "type Good struct"):strings.Index(got, "type Bad struct")]
	firstField := strings.TrimSpace(strings.Split(goodBlock, "\n")[1])
	if !strings.HasPrefix(firstField, "val") {
		t.Errorf("Good struct was incorrectly modified; first field = %q", firstField)
	}
}

func TestStructReorderEditNoOpOnGoodOrder(t *testing.T) {
	src := `package p

type Good struct {
	val  int64
	name string
	flag bool
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c := &checker.StructAlignChecker{}
	issues := c.Check(fset, file)
	if len(issues) != 0 {
		t.Errorf("well-ordered struct should produce no issues, got %d", len(issues))
	}
}

func TestStructReorderEditFourFields(t *testing.T) {
	// {bool, int64, bool, int64} — both bools should end up after both int64s.
	src := `package p

type Quad struct {
	a bool
	b int64
	c bool
	d int64
}
`
	got := applyStructReorderFix(t, src)

	lines := strings.Split(got, "\n")
	var fieldLines []string
	inStruct := false
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "type Quad struct {" {
			inStruct = true
			continue
		}
		if inStruct && trimmed == "}" {
			break
		}
		if inStruct && trimmed != "" {
			fieldLines = append(fieldLines, trimmed)
		}
	}

	if len(fieldLines) != 4 {
		t.Fatalf("expected 4 field lines, got %d: %v", len(fieldLines), fieldLines)
	}
	// First two must be int64, last two must be bool.
	for _, l := range fieldLines[:2] {
		if !strings.Contains(l, "int64") {
			t.Errorf("expected int64 field in first half, got %q", l)
		}
	}
	for _, l := range fieldLines[2:] {
		if !strings.Contains(l, "bool") {
			t.Errorf("expected bool field in second half, got %q", l)
		}
	}
}
