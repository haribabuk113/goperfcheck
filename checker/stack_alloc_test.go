package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestStackAllocChecker_NewPrimitive(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantN   int
		wantMsg string
	}{
		{
			name:    "new(int) triggers",
			src:     `package p; func f() *int { return new(int) }`,
			wantN:   1,
			wantMsg: "new(int)",
		},
		{
			name:    "new(int64) triggers",
			src:     `package p; func f() *int64 { return new(int64) }`,
			wantN:   1,
			wantMsg: "new(int64)",
		},
		{
			name:    "new(bool) triggers",
			src:     `package p; func f() *bool { return new(bool) }`,
			wantN:   1,
			wantMsg: "new(bool)",
		},
		{
			name:    "new(float64) triggers",
			src:     `package p; func f() *float64 { return new(float64) }`,
			wantN:   1,
			wantMsg: "new(float64)",
		},
		{
			name:    "new(byte) triggers",
			src:     `package p; func f() *byte { return new(byte) }`,
			wantN:   1,
			wantMsg: "new(byte)",
		},
		{
			name:    "new(rune) triggers",
			src:     `package p; func f() *rune { return new(rune) }`,
			wantN:   1,
			wantMsg: "new(rune)",
		},
		{
			name:  "new(uint32) triggers",
			src:   `package p; func f() *uint32 { return new(uint32) }`,
			wantN: 1,
		},
		{
			name:  "new(struct) does not trigger",
			src:   `package p; type S struct{ X int }; func f() *S { return new(S) }`,
			wantN: 0,
		},
		{
			name:  "new(string) does not trigger",
			src:   `package p; func f() *string { return new(string) }`,
			wantN: 0,
		},
		{
			name:  "multiple new(primitive) each trigger",
			src:   `package p; func f() (*int, *bool) { return new(int), new(bool) }`,
			wantN: 2,
		},
	}

	c := &StackAllocChecker{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			issues := c.Check(fset, f)
			var newIssues []Issue
			for _, iss := range issues {
				if contains(iss.Message, "new(") {
					newIssues = append(newIssues, iss)
				}
			}
			if len(newIssues) != tt.wantN {
				t.Errorf("got %d new() issue(s), want %d", len(newIssues), tt.wantN)
				for _, iss := range newIssues {
					t.Logf("  issue: %s", iss.Message)
				}
			}
			if tt.wantMsg != "" && len(newIssues) > 0 {
				if !contains(newIssues[0].Message, tt.wantMsg) {
					t.Errorf("message %q does not contain %q", newIssues[0].Message, tt.wantMsg)
				}
			}
		})
	}
}

func TestStackAllocChecker_ReturnAddressOfLocal(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "return &localInt triggers",
			src: `package p
func f() *int {
	var x int
	return &x
}`,
			wantN: 1,
		},
		{
			name: "return &localBool triggers",
			src: `package p
func f() *bool {
	var b bool
	return &b
}`,
			wantN: 1,
		},
		{
			name: "return &localFloat64 triggers",
			src: `package p
func f() *float64 {
	var v float64
	return &v
}`,
			wantN: 1,
		},
		{
			name: "return &localByte triggers",
			src: `package p
func f() *byte {
	var b byte
	return &b
}`,
			wantN: 1,
		},
		{
			name: "return &struct does not trigger",
			src: `package p
type S struct{ X int }
func f() *S {
	var s S
	return &s
}`,
			wantN: 0,
		},
		{
			name: "return value (not pointer) does not trigger",
			src: `package p
func f() int {
	var x int
	return x
}`,
			wantN: 0,
		},
		{
			name: "return &non-local var does not trigger",
			src: `package p
var global int
func f() *int { return &global }`,
			wantN: 0,
		},
	}

	c := &StackAllocChecker{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			issues := c.Check(fset, f)
			var retIssues []Issue
			for _, iss := range issues {
				if contains(iss.Message, "returning &") {
					retIssues = append(retIssues, iss)
				}
			}
			if len(retIssues) != tt.wantN {
				t.Errorf("got %d return-addr issue(s), want %d", len(retIssues), tt.wantN)
				for _, iss := range retIssues {
					t.Logf("  issue: %s", iss.Message)
				}
			}
		})
	}
}

func TestStackAllocChecker_IssueFields(t *testing.T) {
	t.Run("new(primitive) fields", func(t *testing.T) {
		src := `package p; func f() *int { return new(int) }`
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "test.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		c := &StackAllocChecker{}
		issues := c.Check(fset, f)
		var newIssues []Issue
		for _, iss := range issues {
			if contains(iss.Message, "new(") {
				newIssues = append(newIssues, iss)
			}
		}
		if len(newIssues) != 1 {
			t.Fatalf("got %d issue(s), want 1", len(newIssues))
		}
		iss := newIssues[0]
		if iss.Checker != "StackAlloc" {
			t.Errorf("Checker = %q, want StackAlloc", iss.Checker)
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
		if iss.Benchmark == "" {
			t.Error("Benchmark should be non-empty")
		}
	})

	t.Run("return &local fields", func(t *testing.T) {
		src := `package p
func f() *int {
	var x int
	return &x
}`
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "test.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		c := &StackAllocChecker{}
		issues := c.Check(fset, f)
		var retIssues []Issue
		for _, iss := range issues {
			if contains(iss.Message, "returning &") {
				retIssues = append(retIssues, iss)
			}
		}
		if len(retIssues) != 1 {
			t.Fatalf("got %d issue(s), want 1", len(retIssues))
		}
		iss := retIssues[0]
		if iss.Checker != "StackAlloc" {
			t.Errorf("Checker = %q, want StackAlloc", iss.Checker)
		}
		if iss.Severity != SeverityInfo {
			t.Errorf("Severity = %v, want Info", iss.Severity)
		}
		if !contains(iss.Message, "x") {
			t.Errorf("Message %q should name the variable", iss.Message)
		}
	})
}
