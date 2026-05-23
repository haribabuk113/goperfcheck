package checker

import (
	"go/parser"
	"go/token"
	"reflect"
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

func TestStructAlignFixHint(t *testing.T) {
	c := &StructAlignChecker{}

	parse := func(src string) []Issue {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "test.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		return c.Check(fset, f)
	}

	t.Run("fix hint populated on bad struct", func(t *testing.T) {
		issues := parse(`package p
type Bad struct {
	flag bool
	val  int64
	name string
}`)
		if len(issues) != 1 {
			t.Fatalf("got %d issues, want 1", len(issues))
		}
		fix := issues[0].Fix
		if fix == nil {
			t.Fatal("Fix is nil — struct_reorder hint must be populated")
		}
		if fix.Kind != "struct_reorder" {
			t.Errorf("Fix.Kind = %q, want %q", fix.Kind, "struct_reorder")
		}
		if fix.StructName != "Bad" {
			t.Errorf("Fix.StructName = %q, want %q", fix.StructName, "Bad")
		}
		// Fields by approxSize: val int64=8, name string=16, flag bool=1
		// Sorted largest-first: name(idx=2,16B), val(idx=1,8B), flag(idx=0,1B)
		want := []int{2, 1, 0}
		if !reflect.DeepEqual(fix.FieldOrder, want) {
			t.Errorf("Fix.FieldOrder = %v, want %v", fix.FieldOrder, want)
		}
	})

	t.Run("fix not nil for exported struct", func(t *testing.T) {
		issues := parse(`package p
type Exported struct {
	flag bool
	val  int64
	name string
}`)
		if len(issues) != 1 {
			t.Fatalf("got %d issues, want 1", len(issues))
		}
		if issues[0].Fix == nil {
			t.Error("Fix should not be nil for exported struct; user opts in with -fix")
		}
	})

	t.Run("no fix hint on already-sorted struct", func(t *testing.T) {
		// A correctly-ordered struct fires no issue, so Fix is irrelevant.
		issues := parse(`package p
type Good struct {
	val  int64
	name string
	flag bool
}`)
		if len(issues) != 0 {
			t.Errorf("good struct should produce no issues, got %d", len(issues))
		}
	})

	t.Run("fix field order reflects four-field struct", func(t *testing.T) {
		// {bool,int64,bool,int64} — all bools should end up after int64s.
		issues := parse(`package p
type Quad struct {
	a bool
	b int64
	c bool
	d int64
}`)
		if len(issues) != 1 {
			t.Fatalf("got %d issues, want 1", len(issues))
		}
		fix := issues[0].Fix
		if fix == nil {
			t.Fatal("Fix is nil")
		}
		// Sizes: a=1,b=8,c=1,d=8. Sorted stable: b(idx1,8),d(idx3,8),a(idx0,1),c(idx2,1)
		want := []int{1, 3, 0, 2}
		if !reflect.DeepEqual(fix.FieldOrder, want) {
			t.Errorf("Fix.FieldOrder = %v, want %v", fix.FieldOrder, want)
		}
	})
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
