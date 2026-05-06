package checker

import (
	"go/parser"
	"go/token"
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
