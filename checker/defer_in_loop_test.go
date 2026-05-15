package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestDeferInLoopChecker(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "defer in range loop triggers",
			src: `package p
func f(files []string) {
	for _, name := range files {
		f, _ := open(name)
		defer f.Close()
	}
}`,
			wantN: 1,
		},
		{
			name: "defer in c-style for loop triggers",
			src: `package p
func f(n int) {
	for i := 0; i < n; i++ {
		f, _ := open("file")
		defer f.Close()
	}
}`,
			wantN: 1,
		},
		{
			name: "defer outside loop does not trigger",
			src: `package p
func f() {
	f, _ := open("file")
	defer f.Close()
}`,
			wantN: 0,
		},
		{
			name: "multiple defers in same loop each trigger",
			src: `package p
func f(files []string) {
	for _, name := range files {
		f, _ := open(name)
		defer f.Close()
		defer cleanup(name)
	}
}`,
			wantN: 2,
		},
		{
			name: "defer in nested loop triggers once per defer",
			src: `package p
func f(matrix [][]string) {
	for _, row := range matrix {
		for _, name := range row {
			f, _ := open(name)
			defer f.Close()
		}
	}
}`,
			wantN: 1,
		},
		{
			name: "defer in outer and inner loop each trigger",
			src: `package p
func f(matrix [][]string) {
	for _, row := range matrix {
		out, _ := open("outer")
		defer out.Close()
		for _, name := range row {
			f, _ := open(name)
			defer f.Close()
		}
	}
}`,
			wantN: 2,
		},
		{
			name: "defer inside func literal inside loop does not trigger",
			src: `package p
func f(files []string) {
	for _, name := range files {
		func() {
			f, _ := open(name)
			defer f.Close()
		}()
	}
}`,
			wantN: 0,
		},
		{
			name: "defer inside goroutine literal inside loop does not trigger",
			src: `package p
func f(files []string) {
	for _, name := range files {
		go func() {
			f, _ := open(name)
			defer f.Close()
		}()
	}
}`,
			wantN: 0,
		},
		{
			name: "defer inside func literal that itself contains a loop triggers",
			src: `package p
func f(names []string) {
	process := func() {
		for _, name := range names {
			f, _ := open(name)
			defer f.Close()
		}
	}
	process()
}`,
			wantN: 1,
		},
		{
			name: "no loops no defer does not trigger",
			src: `package p
func f() {
	doWork()
}`,
			wantN: 0,
		},
	}

	c := &DeferInLoopChecker{}
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

func TestDeferInLoopChecker_IssueFields(t *testing.T) {
	src := `package p
func f(files []string) {
	for _, name := range files {
		f, _ := open(name)
		defer f.Close()
	}
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	c := &DeferInLoopChecker{}
	issues := c.Check(fset, f)
	if len(issues) != 1 {
		t.Fatalf("got %d issue(s), want 1", len(issues))
	}
	iss := issues[0]
	if iss.Checker != "DeferInLoop" {
		t.Errorf("Checker = %q, want DeferInLoop", iss.Checker)
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
	if iss.Suggestion == "" {
		t.Error("Suggestion should be non-empty")
	}
	if iss.Benchmark == "" {
		t.Error("Benchmark should be non-empty")
	}
}
