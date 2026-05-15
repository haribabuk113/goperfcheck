package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestStringConcatLoopChecker(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "s += x in range loop triggers",
			src: `package p
func f(words []string) string {
	var s string
	for _, w := range words {
		s += w
	}
	return s
}`,
			wantN: 1,
		},
		{
			name: "s += x in c-style for loop triggers",
			src: `package p
func f(n int) string {
	var s string
	for i := 0; i < n; i++ {
		s += "x"
	}
	return s
}`,
			wantN: 1,
		},
		{
			name: "s = s + x in range loop triggers",
			src: `package p
func f(words []string) string {
	var s string
	for _, w := range words {
		s = s + w
	}
	return s
}`,
			wantN: 1,
		},
		{
			name: "s = x + s in range loop triggers",
			src: `package p
func f(words []string) string {
	var s string
	for _, w := range words {
		s = w + s
	}
	return s
}`,
			wantN: 1,
		},
		{
			name: "s += x outside loop does not trigger",
			src: `package p
func f(a, b string) string {
	s := a
	s += b
	return s
}`,
			wantN: 0,
		},
		{
			// The checker is AST-only and has no type info: += on any single-ident
			// LHS inside a loop triggers, including int. This is a known false positive.
			name: "unrelated += (int) in loop triggers (known false positive — no type info)",
			src: `package p
func f(n int) int {
	sum := 0
	for i := 0; i < n; i++ {
		sum += i
	}
	return sum
}`,
			wantN: 1,
		},
		{
			name: "a = b + c where a is different from b and c does not trigger",
			src: `package p
func f(words []string) string {
	var a, b, c string
	for _, w := range words {
		a = b + c
		_ = w
	}
	return a
}`,
			wantN: 0,
		},
		{
			name: "s += x inside func literal inside loop does not trigger",
			src: `package p
func f(words []string) {
	for _, w := range words {
		fn := func() string {
			var s string
			s += w
			return s
		}
		_ = fn
	}
}`,
			wantN: 0,
		},
		{
			name: "s += x inside func literal that contains a loop triggers",
			src: `package p
func f(words []string) func() string {
	return func() string {
		var s string
		for _, w := range words {
			s += w
		}
		return s
	}
}`,
			wantN: 1,
		},
		{
			name: "multiple string concat ops in same loop each trigger",
			src: `package p
func f(words []string) (string, string) {
	var a, b string
	for _, w := range words {
		a += w
		b += w
	}
	return a, b
}`,
			wantN: 2,
		},
	}

	c := &StringConcatLoopChecker{}
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
					t.Logf("  issue: %s (line %d)", iss.Message, iss.Line)
				}
			}
		})
	}
}

func TestStringConcatLoopChecker_IssueFields(t *testing.T) {
	src := `package p
func f(words []string) string {
	var s string
	for _, w := range words {
		s += w
	}
	return s
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	c := &StringConcatLoopChecker{}
	issues := c.Check(fset, f)
	if len(issues) != 1 {
		t.Fatalf("got %d issue(s), want 1", len(issues))
	}
	iss := issues[0]
	if iss.Checker != "StringConcatLoop" {
		t.Errorf("Checker = %q, want StringConcatLoop", iss.Checker)
	}
	if iss.Severity != SeverityWarning {
		t.Errorf("Severity = %v, want Warning", iss.Severity)
	}
	if iss.Line == 0 {
		t.Error("Line should be non-zero")
	}
	if !contains(iss.Rule, "goperf.dev") {
		t.Errorf("Rule %q missing goperf.dev link", iss.Rule)
	}
	if !contains(iss.Suggestion, "strings.Builder") {
		t.Errorf("Suggestion %q should mention strings.Builder", iss.Suggestion)
	}
	if iss.Benchmark == "" {
		t.Error("Benchmark should be non-empty")
	}
}
