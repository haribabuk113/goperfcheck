package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestMemPreallocChecker(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "append in range loop triggers",
			src: `package p
func f(items []string) []string {
	var result []string
	for _, v := range items { result = append(result, v) }
	return result
}`,
			wantN: 1,
		},
		{
			name: "append in c-style for loop triggers",
			src: `package p
func f(n int) []int {
	var out []int
	for i := 0; i < n; i++ { out = append(out, i) }
	return out
}`,
			wantN: 1,
		},
		{
			name: "append outside loop does not trigger",
			src: `package p
func f() []string {
	var s []string
	s = append(s, "a")
	return s
}`,
			wantN: 0,
		},
		{
			name: "make map without hint triggers",
			src: `package p
func f() map[string]int { return make(map[string]int) }`,
			wantN: 1,
		},
		{
			name: "make map with hint does not trigger",
			src: `package p
func f() map[string]int { return make(map[string]int, 16) }`,
			wantN: 0,
		},
		{
			name: "range hint inferred for map",
			src: `package p
func f(items []string) map[string]int {
	m := make(map[string]int)
	for i, v := range items { m[v] = i }
	return m
}`,
			wantN: 1,
			// Hint should be len(items) — verified by checking suggestion in a separate test.
		},
	}

	c := &MemPreallocChecker{}
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
				for _, iss := range issues {
					t.Logf("  issue: %s", iss.Message)
				}
			}
		})
	}
}

func TestMemPreallocHints(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantHint string
	}{
		{
			name: "range loop hint",
			src: `package p
func f(items []string) []string {
	var r []string
	for _, v := range items { r = append(r, v) }
	return r
}`,
			wantHint: "len(items)",
		},
		{
			name: "c-style loop hint",
			src: `package p
func f(n int) []int {
	var r []int
	for i := 0; i < n; i++ { r = append(r, i) }
	return r
}`,
			wantHint: "n",
		},
		{
			name: "c-style <= loop hint",
			src: `package p
func f(n int) []int {
	var r []int
	for i := 0; i <= n; i++ { r = append(r, i) }
	return r
}`,
			wantHint: "n+1",
		},
		{
			name: "map hint from subsequent range",
			src: `package p
func f(items []string) map[string]int {
	m := make(map[string]int)
	for i, v := range items { m[v] = i }
	return m
}`,
			wantHint: "len(items)",
		},
		{
			name: "map fallback default hint",
			src: `package p
func f() map[string]int { return make(map[string]int) }`,
			wantHint: "8",
		},
	}

	c := &MemPreallocChecker{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			issues := c.Check(fset, f)
			if len(issues) == 0 {
				t.Fatal("expected at least one issue, got none")
			}
			sug := issues[0].Suggestion
			if !contains(sug, tt.wantHint) {
				t.Errorf("suggestion %q does not contain hint %q", sug, tt.wantHint)
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
