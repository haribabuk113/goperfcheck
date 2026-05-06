package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestContextMisuseChecker(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "context stored in struct field triggers",
			src: `package p
import "context"
type Server struct {
	ctx context.Context
}`,
			wantN: 1,
		},
		{
			name: "context as function parameter does not trigger",
			src: `package p
import "context"
func Handle(ctx context.Context) {}`,
			wantN: 0,
		},
		{
			name: "non-context struct field does not trigger",
			src: `package p
type Server struct {
	name string
	port int
}`,
			wantN: 0,
		},
		{
			name: "multiple context fields each trigger",
			src: `package p
import "context"
type Worker struct {
	ctx1 context.Context
	ctx2 context.Context
}`,
			wantN: 2,
		},
	}

	c := &ContextMisuseChecker{}
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
