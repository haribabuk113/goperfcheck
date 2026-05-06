package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestGoroutinePoolChecker(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "go statement in range loop triggers",
			src: `package p
func f(items []string) {
	for _, v := range items {
		go func(s string) {}(v)
	}
}`,
			wantN: 1,
		},
		{
			name: "go statement in for loop triggers",
			src: `package p
func f(n int) {
	for i := 0; i < n; i++ {
		go func() {}()
	}
}`,
			wantN: 1,
		},
		{
			name: "go statement outside loop does not trigger",
			src: `package p
func f() {
	go func() {}()
}`,
			wantN: 0,
		},
		{
			name: "no goroutines does not trigger",
			src: `package p
func f(items []string) {
	for _, v := range items { _ = v }
}`,
			wantN: 0,
		},
	}

	c := &GoroutinePoolChecker{}
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
