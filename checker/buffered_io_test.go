package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestBufferedIOChecker_UnbufferedWriteInLoop(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "Write in range loop triggers",
			src: `package p
func f(w Writer, lines []string) {
	for _, line := range lines {
		w.Write([]byte(line))
	}
}`,
			wantN: 1,
		},
		{
			name: "WriteString in range loop triggers",
			src: `package p
func f(w Writer, lines []string) {
	for _, line := range lines {
		w.WriteString(line)
	}
}`,
			wantN: 1,
		},
		{
			name: "WriteByte in for loop triggers",
			src: `package p
func f(w Writer, data []byte) {
	for i := 0; i < len(data); i++ {
		w.WriteByte(data[i])
	}
}`,
			wantN: 1,
		},
		{
			name: "WriteRune in range loop triggers",
			src: `package p
func f(w Writer, chars []rune) {
	for _, ch := range chars {
		w.WriteRune(ch)
	}
}`,
			wantN: 1,
		},
		{
			name: "Write outside loop does not trigger",
			src: `package p
func f(w Writer, data []byte) {
	w.Write(data)
}`,
			wantN: 0,
		},
		{
			name: "non-write method in loop does not trigger",
			src: `package p
func f(w Writer, lines []string) {
	for _, line := range lines {
		w.Close()
		_ = line
	}
}`,
			wantN: 0,
		},
		{
			name: "multiple write methods in same loop each trigger",
			src: `package p
func f(w Writer, lines []string) {
	for _, line := range lines {
		w.Write([]byte(line))
		w.WriteByte('\n')
	}
}`,
			wantN: 2,
		},
		{
			name: "write on chained selector does not trigger",
			src: `package p
func f(lines []string) {
	for _, line := range lines {
		obj.w.Write([]byte(line))
	}
}`,
			wantN: 0,
		},
	}

	c := &BufferedIOChecker{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			issues := c.Check(fset, f)
			// Only count write-in-loop issues (SeverityWarning), not flush issues.
			var writeIssues []Issue
			for _, iss := range issues {
				if iss.Severity == SeverityWarning {
					writeIssues = append(writeIssues, iss)
				}
			}
			if len(writeIssues) != tt.wantN {
				t.Errorf("got %d write-in-loop issue(s), want %d", len(writeIssues), tt.wantN)
				for _, iss := range writeIssues {
					t.Logf("  issue: %s", iss.Message)
				}
			}
		})
	}
}

func TestBufferedIOChecker_MissingFlush(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "bufio.NewWriter without Flush triggers",
			src: `package p
import "bufio"
func f(w Writer) {
	bw := bufio.NewWriter(w)
	bw.WriteString("hello")
}`,
			wantN: 1,
		},
		{
			name: "bufio.NewWriter with Flush does not trigger",
			src: `package p
import "bufio"
func f(w Writer) {
	bw := bufio.NewWriter(w)
	bw.WriteString("hello")
	bw.Flush()
}`,
			wantN: 0,
		},
		{
			name: "bufio.NewWriter with defer Flush does not trigger",
			src: `package p
import "bufio"
func f(w Writer) {
	bw := bufio.NewWriter(w)
	defer bw.Flush()
	bw.WriteString("hello")
}`,
			wantN: 0,
		},
		{
			name: "bufio.NewReader without Flush triggers",
			src: `package p
import "bufio"
func f(r Reader) {
	br := bufio.NewReader(r)
	_ = br
}`,
			wantN: 1,
		},
		{
			name: "two bufio.NewWriter in same function both trigger when no Flush",
			src: `package p
import "bufio"
func f(w1 Writer, w2 Writer) {
	bw1 := bufio.NewWriter(w1)
	bw2 := bufio.NewWriter(w2)
	bw1.WriteString("a")
	bw2.WriteString("b")
}`,
			wantN: 2,
		},
		{
			name: "two bufio.NewWriter in same function — Flush on one suppresses both",
			src: `package p
import "bufio"
func f(w1 Writer, w2 Writer) {
	bw1 := bufio.NewWriter(w1)
	bw2 := bufio.NewWriter(w2)
	bw1.WriteString("a")
	bw2.WriteString("b")
	bw1.Flush()
}`,
			// hasFlushed is a single bool — any Flush() in the function suppresses all.
			wantN: 0,
		},
		{
			name: "bufio.NewWriter in one function and Flush in another — still triggers",
			src: `package p
import "bufio"
func write(w Writer) {
	bw := bufio.NewWriter(w)
	bw.WriteString("hello")
}
func flush(bw interface{ Flush() error }) {
	bw.Flush()
}`,
			wantN: 1,
		},
		{
			name: "no bufio usage does not trigger",
			src: `package p
func f(w Writer) {
	w.Write([]byte("hello"))
}`,
			wantN: 0,
		},
	}

	c := &BufferedIOChecker{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			issues := c.Check(fset, f)
			// Only count missing-flush issues (SeverityError).
			var flushIssues []Issue
			for _, iss := range issues {
				if iss.Severity == SeverityError {
					flushIssues = append(flushIssues, iss)
				}
			}
			if len(flushIssues) != tt.wantN {
				t.Errorf("got %d missing-flush issue(s), want %d", len(flushIssues), tt.wantN)
				for _, iss := range flushIssues {
					t.Logf("  issue: %s", iss.Message)
				}
			}
		})
	}
}

func TestBufferedIOChecker_IssueFields(t *testing.T) {
	t.Run("write-in-loop fields", func(t *testing.T) {
		src := `package p
func f(w Writer, lines []string) {
	for _, line := range lines {
		w.WriteString(line)
	}
}`
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "test.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		c := &BufferedIOChecker{}
		issues := c.Check(fset, f)
		if len(issues) != 1 {
			t.Fatalf("got %d issue(s), want 1", len(issues))
		}
		iss := issues[0]
		if iss.Checker != "BufferedIO" {
			t.Errorf("Checker = %q, want BufferedIO", iss.Checker)
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
	})

	t.Run("missing-flush fields", func(t *testing.T) {
		src := `package p
import "bufio"
func f(w Writer) {
	bw := bufio.NewWriter(w)
	bw.WriteString("hello")
}`
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "test.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		c := &BufferedIOChecker{}
		issues := c.Check(fset, f)
		var flushIssues []Issue
		for _, iss := range issues {
			if iss.Severity == SeverityError {
				flushIssues = append(flushIssues, iss)
			}
		}
		if len(flushIssues) != 1 {
			t.Fatalf("got %d missing-flush issue(s), want 1", len(flushIssues))
		}
		iss := flushIssues[0]
		if iss.Checker != "BufferedIO" {
			t.Errorf("Checker = %q, want BufferedIO", iss.Checker)
		}
		if !contains(iss.Message, "bw") {
			t.Errorf("Message %q should mention variable name %q", iss.Message, "bw")
		}
		if !contains(iss.Suggestion, "bw.Flush()") {
			t.Errorf("Suggestion %q should include %q", iss.Suggestion, "bw.Flush()")
		}
		if !contains(iss.Rule, "goperf.dev") {
			t.Errorf("Rule %q missing goperf.dev link", iss.Rule)
		}
	})
}
