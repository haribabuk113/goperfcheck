package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestZeroCopyChecker_AppendByteSlice(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "append([]byte{}, src...) triggers",
			src: `package p
func f(src []byte) []byte {
	return append([]byte{}, src...)
}`,
			wantN: 1,
		},
		{
			name: "append([]byte{}, src...) inside function triggers",
			src: `package p
func clone(src []byte) []byte {
	dst := append([]byte{}, src...)
	return dst
}`,
			wantN: 1,
		},
		{
			name: "append([]byte{}, src...) inside loop triggers",
			src: `package p
func f(chunks [][]byte) [][]byte {
	var out [][]byte
	for _, chunk := range chunks {
		out = append(out, append([]byte{}, chunk...))
	}
	return out
}`,
			wantN: 1,
		},
		{
			name: "append to non-empty slice does not trigger",
			src: `package p
func f(dst, src []byte) []byte {
	return append(dst, src...)
}`,
			wantN: 0,
		},
		{
			// The checker matches any append([]byte{}, args...) where the first arg
			// is an empty []byte{} literal — it does not require the spread operator.
			name: "append([]byte{}, single_elem) without spread also triggers",
			src: `package p
func f(b byte) []byte {
	return append([]byte{}, b)
}`,
			wantN: 1,
		},
		{
			name: "append([]int{}, src...) (not []byte) does not trigger",
			src: `package p
func f(src []int) []int {
	return append([]int{}, src...)
}`,
			wantN: 0,
		},
		{
			name: "append([]byte{1,2}, src...) (non-empty literal) does not trigger",
			src: `package p
func f(src []byte) []byte {
	return append([]byte{1, 2}, src...)
}`,
			wantN: 0,
		},
		{
			name: "multiple append([]byte{}, ...) each trigger",
			src: `package p
func f(a, b []byte) ([]byte, []byte) {
	return append([]byte{}, a...), append([]byte{}, b...)
}`,
			wantN: 2,
		},
	}

	c := &ZeroCopyChecker{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			issues := c.Check(fset, f)
			var appendIssues []Issue
			for _, iss := range issues {
				if contains(iss.Message, "append([]byte{}") {
					appendIssues = append(appendIssues, iss)
				}
			}
			if len(appendIssues) != tt.wantN {
				t.Errorf("got %d append issue(s), want %d", len(appendIssues), tt.wantN)
				for _, iss := range appendIssues {
					t.Logf("  issue: %s", iss.Message)
				}
			}
		})
	}
}

func TestZeroCopyChecker_CopyInLoop(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "copy() in range loop triggers",
			src: `package p
func f(chunks [][]byte) {
	dst := make([]byte, 1024)
	for _, chunk := range chunks {
		copy(dst, chunk)
	}
}`,
			wantN: 1,
		},
		{
			name: "copy() in c-style for loop triggers",
			src: `package p
func f(src []byte, n int) {
	buf := make([]byte, 64)
	for i := 0; i < n; i++ {
		copy(buf, src[i*64:])
	}
}`,
			wantN: 1,
		},
		{
			name: "copy() outside loop does not trigger",
			src: `package p
func f(dst, src []byte) {
	copy(dst, src)
}`,
			wantN: 0,
		},
		{
			name: "multiple copy() calls in same loop each trigger",
			src: `package p
func f(chunks [][]byte, dst1, dst2 []byte) {
	for _, chunk := range chunks {
		copy(dst1, chunk)
		copy(dst2, chunk)
	}
}`,
			wantN: 2,
		},
	}

	c := &ZeroCopyChecker{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			issues := c.Check(fset, f)
			var copyIssues []Issue
			for _, iss := range issues {
				if contains(iss.Message, "copy() inside a loop") {
					copyIssues = append(copyIssues, iss)
				}
			}
			if len(copyIssues) != tt.wantN {
				t.Errorf("got %d copy-in-loop issue(s), want %d", len(copyIssues), tt.wantN)
				for _, iss := range copyIssues {
					t.Logf("  issue: %s", iss.Message)
				}
			}
		})
	}
}

func TestZeroCopyChecker_IoCopyInLoop(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "io.Copy in range loop triggers",
			src: `package p
import "io"
func f(readers []io.Reader, w io.Writer) {
	for _, r := range readers {
		io.Copy(w, r)
	}
}`,
			wantN: 1,
		},
		{
			name: "io.Copy in c-style for loop triggers",
			src: `package p
import "io"
func f(r io.Reader, w io.Writer, n int) {
	for i := 0; i < n; i++ {
		io.Copy(w, r)
	}
}`,
			wantN: 1,
		},
		{
			name: "io.Copy outside loop does not trigger",
			src: `package p
import "io"
func f(r io.Reader, w io.Writer) {
	io.Copy(w, r)
}`,
			wantN: 0,
		},
		{
			name: "io.CopyBuffer in loop does not trigger",
			src: `package p
import "io"
func f(readers []io.Reader, w io.Writer, buf []byte) {
	for _, r := range readers {
		io.CopyBuffer(w, r, buf)
	}
}`,
			wantN: 0,
		},
	}

	c := &ZeroCopyChecker{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			issues := c.Check(fset, f)
			var ioIssues []Issue
			for _, iss := range issues {
				if contains(iss.Message, "io.Copy()") {
					ioIssues = append(ioIssues, iss)
				}
			}
			if len(ioIssues) != tt.wantN {
				t.Errorf("got %d io.Copy issue(s), want %d", len(ioIssues), tt.wantN)
				for _, iss := range ioIssues {
					t.Logf("  issue: %s", iss.Message)
				}
			}
		})
	}
}

func TestZeroCopyChecker_IssueFields(t *testing.T) {
	t.Run("append([]byte{}) fields", func(t *testing.T) {
		src := `package p
func f(src []byte) []byte { return append([]byte{}, src...) }`
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "test.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		c := &ZeroCopyChecker{}
		issues := c.Check(fset, f)
		var appendIssues []Issue
		for _, iss := range issues {
			if contains(iss.Message, "append([]byte{}") {
				appendIssues = append(appendIssues, iss)
			}
		}
		if len(appendIssues) != 1 {
			t.Fatalf("got %d issue(s), want 1", len(appendIssues))
		}
		iss := appendIssues[0]
		if iss.Checker != "ZeroCopy" {
			t.Errorf("Checker = %q, want ZeroCopy", iss.Checker)
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

	t.Run("io.Copy-in-loop fields", func(t *testing.T) {
		src := `package p
import "io"
func f(readers []io.Reader, w io.Writer) {
	for _, r := range readers {
		io.Copy(w, r)
	}
}`
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "test.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		c := &ZeroCopyChecker{}
		issues := c.Check(fset, f)
		var ioIssues []Issue
		for _, iss := range issues {
			if contains(iss.Message, "io.Copy()") {
				ioIssues = append(ioIssues, iss)
			}
		}
		if len(ioIssues) != 1 {
			t.Fatalf("got %d issue(s), want 1", len(ioIssues))
		}
		iss := ioIssues[0]
		if iss.Checker != "ZeroCopy" {
			t.Errorf("Checker = %q, want ZeroCopy", iss.Checker)
		}
		if iss.Severity != SeverityInfo {
			t.Errorf("Severity = %v, want Info", iss.Severity)
		}
		if !contains(iss.Suggestion, "io.CopyBuffer") {
			t.Errorf("Suggestion %q should mention io.CopyBuffer", iss.Suggestion)
		}
	})
}
