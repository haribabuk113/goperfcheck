package checker

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestStructAlignChecker(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "small field before large field triggers",
			src: `package p
type Bad struct {
	flag bool
	val  int64
	name string
}`,
			wantN: 1,
		},
		{
			name: "large field before small field does not trigger",
			src: `package p
type Good struct {
	val  int64
	name string
	flag bool
}`,
			wantN: 0,
		},
		{
			name: "all same-size fields do not trigger",
			src: `package p
type Equal struct {
	a int64
	b int64
	c int64
}`,
			wantN: 0,
		},
		{
			name: "fewer than 3 fields not checked",
			src: `package p
type Small struct {
	flag bool
	val  int64
}`,
			wantN: 0,
		},
	}

	c := &StructAlignChecker{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			issues := c.Check(fset, f)
			if len(issues) != tt.wantN {
				t.Errorf("got %d issue(s), want %d", len(issues), tt.wantN)
			}
		})
	}
}

func TestStructAlignExportedWarning(t *testing.T) {
	c := &StructAlignChecker{}

	exportedSrc := `package p
type Request struct {
	ok   bool
	id   int64
	name string
}`

	unexportedSrc := `package p
type request struct {
	ok   bool
	id   int64
	name string
}`

	parse := func(src string) []Issue {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "test.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		return c.Check(fset, f)
	}

	t.Run("exported struct carries API-safety caveat in suggestion", func(t *testing.T) {
		issues := parse(exportedSrc)
		if len(issues) != 1 {
			t.Fatalf("got %d issue(s), want 1", len(issues))
		}
		sug := issues[0].Suggestion
		if !strings.Contains(sug, "Exported type") {
			t.Errorf("suggestion for exported struct missing API-safety caveat; got: %q", sug)
		}
		if !strings.Contains(sug, "positional struct literals") {
			t.Errorf("suggestion for exported struct missing positional-literal mention; got: %q", sug)
		}
		if !strings.Contains(sug, "breaking API change") {
			t.Errorf("suggestion for exported struct missing breaking-change warning; got: %q", sug)
		}
	})

	t.Run("unexported struct uses standard reorder suggestion", func(t *testing.T) {
		issues := parse(unexportedSrc)
		if len(issues) != 1 {
			t.Fatalf("got %d issue(s), want 1", len(issues))
		}
		sug := issues[0].Suggestion
		if strings.Contains(sug, "Exported type") {
			t.Errorf("suggestion for unexported struct should not mention exported type; got: %q", sug)
		}
		if !strings.Contains(sug, "Reorder fields") {
			t.Errorf("suggestion for unexported struct should contain reorder guidance; got: %q", sug)
		}
	})
}
