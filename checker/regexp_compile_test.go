package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestRegexpCompileChecker(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "regexp.Compile inside function triggers",
			src: `package p
import "regexp"
func f(s string) bool {
	re, _ := regexp.Compile("foo.*bar")
	return re.MatchString(s)
}`,
			wantN: 1,
		},
		{
			name: "regexp.MustCompile inside function triggers",
			src: `package p
import "regexp"
func f(s string) bool {
	re := regexp.MustCompile("foo.*bar")
	return re.MatchString(s)
}`,
			wantN: 1,
		},
		{
			name: "regexp.CompilePOSIX inside function triggers",
			src: `package p
import "regexp"
func f(s string) bool {
	re, _ := regexp.CompilePOSIX("foo.*bar")
	return re.MatchString(s)
}`,
			wantN: 1,
		},
		{
			name: "regexp.MustCompilePOSIX inside function triggers",
			src: `package p
import "regexp"
func f(s string) bool {
	re := regexp.MustCompilePOSIX("foo.*bar")
	return re.MatchString(s)
}`,
			wantN: 1,
		},
		{
			name: "regexp.MustCompile at package level does not trigger",
			src: `package p
import "regexp"
var re = regexp.MustCompile("foo.*bar")
func f(s string) bool { return re.MatchString(s) }`,
			wantN: 0,
		},
		{
			name: "regexp.Compile at package level does not trigger",
			src: `package p
import "regexp"
var re, _ = regexp.Compile("foo.*bar")`,
			wantN: 0,
		},
		{
			name: "regexp.MustCompile inside func literal triggers",
			src: `package p
import "regexp"
func f() {
	fn := func(s string) bool {
		re := regexp.MustCompile("foo")
		return re.MatchString(s)
	}
	_ = fn
}`,
			wantN: 1,
		},
		{
			name: "regexp.MustCompile inside goroutine literal triggers",
			src: `package p
import "regexp"
func f(s string) {
	go func() {
		re := regexp.MustCompile("foo")
		_ = re.MatchString(s)
	}()
}`,
			wantN: 1,
		},
		{
			name: "multiple regexp calls in same function each trigger",
			src: `package p
import "regexp"
func f(s string) {
	re1 := regexp.MustCompile("foo")
	re2 := regexp.MustCompile("bar")
	_ = re1.MatchString(s)
	_ = re2.MatchString(s)
}`,
			wantN: 2,
		},
		{
			name: "regexp.MatchString (not Compile) does not trigger",
			src: `package p
import "regexp"
func f(s string) (bool, error) {
	return regexp.MatchString("foo", s)
}`,
			wantN: 0,
		},
		{
			name: "no regexp usage does not trigger",
			src: `package p
func f(s string) string { return s }`,
			wantN: 0,
		},
	}

	c := &RegexpCompileChecker{}
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

func TestRegexpCompileChecker_IssueFields(t *testing.T) {
	src := `package p
import "regexp"
func f(s string) bool {
	re := regexp.MustCompile("foo.*bar")
	return re.MatchString(s)
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	c := &RegexpCompileChecker{}
	issues := c.Check(fset, f)
	if len(issues) != 1 {
		t.Fatalf("got %d issue(s), want 1", len(issues))
	}
	iss := issues[0]
	if iss.Checker != "RegexpCompile" {
		t.Errorf("Checker = %q, want RegexpCompile", iss.Checker)
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
	if !contains(iss.Suggestion, "package-level") {
		t.Errorf("Suggestion %q should mention package-level", iss.Suggestion)
	}
	if iss.Benchmark == "" {
		t.Error("Benchmark should be non-empty")
	}
}
