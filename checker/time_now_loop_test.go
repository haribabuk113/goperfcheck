package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestTimeNowLoopChecker(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "time.Now() in range loop triggers",
			src: `package p
import "time"
func f(items []string) {
	for _, item := range items {
		t := time.Now()
		_ = t
		_ = item
	}
}`,
			wantN: 1,
		},
		{
			name: "time.Now() in c-style for loop triggers",
			src: `package p
import "time"
func f(n int) {
	for i := 0; i < n; i++ {
		t := time.Now()
		_ = t
	}
}`,
			wantN: 1,
		},
		{
			name: "time.Now() outside loop does not trigger",
			src: `package p
import "time"
func f() time.Time {
	return time.Now()
}`,
			wantN: 0,
		},
		{
			name: "multiple time.Now() in same loop body each trigger",
			src: `package p
import "time"
func f(n int) {
	for i := 0; i < n; i++ {
		start := time.Now()
		end := time.Now()
		_ = start
		_ = end
	}
}`,
			wantN: 2,
		},
		{
			name: "time.Now() in outer loop only — inner loop stops recursion",
			src: `package p
import "time"
func f(n int) {
	for i := 0; i < n; i++ {
		t := time.Now()
		_ = t
		for j := 0; j < n; j++ {
			_ = j
		}
	}
}`,
			wantN: 1,
		},
		{
			name: "time.Now() in inner loop triggers attributed to inner",
			src: `package p
import "time"
func f(n int) {
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			t := time.Now()
			_ = t
		}
	}
}`,
			wantN: 1,
		},
		{
			name: "time.Now() in both outer and inner loop triggers twice",
			src: `package p
import "time"
func f(n int) {
	for i := 0; i < n; i++ {
		outer := time.Now()
		_ = outer
		for j := 0; j < n; j++ {
			inner := time.Now()
			_ = inner
		}
	}
}`,
			wantN: 2,
		},
		{
			name: "time.Since inside loop does not trigger (not time.Now)",
			src: `package p
import "time"
func f(items []string, start time.Time) {
	for _, item := range items {
		_ = time.Since(start)
		_ = item
	}
}`,
			wantN: 0,
		},
		{
			name: "no time package usage does not trigger",
			src: `package p
func f(n int) {
	for i := 0; i < n; i++ {
		doWork(i)
	}
}`,
			wantN: 0,
		},
	}

	c := TimeNowLoopChecker{}
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

func TestTimeNowLoopChecker_IssueFields(t *testing.T) {
	src := `package p
import "time"
func f(items []string) {
	for _, item := range items {
		t := time.Now()
		_ = t
		_ = item
	}
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	c := TimeNowLoopChecker{}
	issues := c.Check(fset, f)
	if len(issues) != 1 {
		t.Fatalf("got %d issue(s), want 1", len(issues))
	}
	iss := issues[0]
	if iss.Checker != "TimeNowLoop" {
		t.Errorf("Checker = %q, want TimeNowLoop", iss.Checker)
	}
	if iss.Severity != SeverityInfo {
		t.Errorf("Severity = %v, want Info", iss.Severity)
	}
	if iss.Line == 0 {
		t.Error("Line should be non-zero")
	}
	if !contains(iss.Rule, "goperf.dev") {
		t.Errorf("Rule %q missing goperf.dev link", iss.Rule)
	}
	if !contains(iss.Message, "time.Now()") {
		t.Errorf("Message %q should mention time.Now()", iss.Message)
	}
	if iss.Benchmark == "" {
		t.Error("Benchmark should be non-empty")
	}
}
