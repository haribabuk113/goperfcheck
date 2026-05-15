package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestObjectPoolChecker_PoolableCallsInLoop(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "bytes.NewBuffer in range loop triggers",
			src: `package p
import "bytes"
func f(items []string) {
	for _, s := range items {
		buf := bytes.NewBuffer(nil)
		buf.WriteString(s)
	}
}`,
			wantN: 1,
		},
		{
			name: "bytes.NewBufferString in range loop triggers",
			src: `package p
import "bytes"
func f(items []string) {
	for _, s := range items {
		buf := bytes.NewBufferString(s)
		_ = buf
	}
}`,
			wantN: 1,
		},
		{
			name: "strings.NewReader in range loop triggers",
			src: `package p
import "strings"
func f(items []string) {
	for _, s := range items {
		r := strings.NewReader(s)
		_ = r
	}
}`,
			wantN: 1,
		},
		{
			name: "bufio.NewWriter in range loop triggers",
			src: `package p
import "bufio"
func f(writers []Writer, items []string) {
	for i, w := range writers {
		bw := bufio.NewWriter(w)
		bw.WriteString(items[i])
		bw.Flush()
	}
}`,
			wantN: 1,
		},
		{
			name: "bufio.NewReader in range loop triggers",
			src: `package p
import "bufio"
func f(readers []Reader) {
	for _, r := range readers {
		br := bufio.NewReader(r)
		_ = br
	}
}`,
			wantN: 1,
		},
		{
			name: "json.NewEncoder in range loop triggers",
			src: `package p
import "encoding/json"
func f(writers []Writer, items []Item) {
	for i, w := range writers {
		enc := json.NewEncoder(w)
		enc.Encode(items[i])
	}
}`,
			wantN: 1,
		},
		{
			name: "json.NewDecoder in range loop triggers",
			src: `package p
import "encoding/json"
func f(readers []Reader) {
	for _, r := range readers {
		dec := json.NewDecoder(r)
		_ = dec
	}
}`,
			wantN: 1,
		},
		{
			name: "bytes.NewBuffer outside loop does not trigger",
			src: `package p
import "bytes"
func f() {
	buf := bytes.NewBuffer(nil)
	buf.WriteString("hello")
}`,
			wantN: 0,
		},
		{
			name: "unknown package call in loop does not trigger",
			src: `package p
func f(items []string) {
	for _, s := range items {
		buf := myPkg.NewBuffer(nil)
		_ = buf
		_ = s
	}
}`,
			wantN: 0,
		},
		{
			name: "multiple poolable calls in same loop each trigger",
			src: `package p
import (
	"bytes"
	"strings"
)
func f(items []string) {
	for _, s := range items {
		buf := bytes.NewBuffer(nil)
		r := strings.NewReader(s)
		_ = buf
		_ = r
	}
}`,
			wantN: 2,
		},
	}

	c := &ObjectPoolChecker{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			issues := c.Check(fset, f)
			// Only count poolable-call issues (message contains "called inside a loop")
			var poolIssues []Issue
			for _, iss := range issues {
				if contains(iss.Message, "called inside a loop") {
					poolIssues = append(poolIssues, iss)
				}
			}
			if len(poolIssues) != tt.wantN {
				t.Errorf("got %d issue(s), want %d", len(poolIssues), tt.wantN)
				for _, iss := range poolIssues {
					t.Logf("  issue: %s", iss.Message)
				}
			}
		})
	}
}

func TestObjectPoolChecker_MakeByteSliceInLoop(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "make([]byte, n) in range loop triggers",
			src: `package p
func f(items []string) {
	for _, s := range items {
		buf := make([]byte, len(s))
		copy(buf, s)
	}
}`,
			wantN: 1,
		},
		{
			name: "make([]byte, n, cap) in range loop triggers",
			src: `package p
func f(n int) {
	for i := 0; i < n; i++ {
		buf := make([]byte, 512, 1024)
		_ = buf
	}
}`,
			wantN: 1,
		},
		{
			name: "make([]byte, n) outside loop does not trigger",
			src: `package p
func f(n int) []byte {
	return make([]byte, n)
}`,
			wantN: 0,
		},
		{
			name: "make([]int, n) in loop does not trigger (not []byte)",
			src: `package p
func f(n int) {
	for i := 0; i < n; i++ {
		buf := make([]int, 10)
		_ = buf
	}
}`,
			wantN: 0,
		},
		{
			name: "make([]byte) with no length arg does not trigger",
			src: `package p
func f(n int) {
	for i := 0; i < n; i++ {
		buf := make([]byte)
		_ = buf
	}
}`,
			wantN: 0,
		},
		{
			name: "[N]byte fixed-size array in loop does not trigger",
			src: `package p
func f(n int) {
	for i := 0; i < n; i++ {
		var buf [512]byte
		_ = buf
	}
}`,
			wantN: 0,
		},
	}

	c := &ObjectPoolChecker{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			issues := c.Check(fset, f)
			var byteIssues []Issue
			for _, iss := range issues {
				if contains(iss.Message, "make([]byte") {
					byteIssues = append(byteIssues, iss)
				}
			}
			if len(byteIssues) != tt.wantN {
				t.Errorf("got %d make([]byte) issue(s), want %d", len(byteIssues), tt.wantN)
				for _, iss := range byteIssues {
					t.Logf("  issue: %s", iss.Message)
				}
			}
		})
	}
}

func TestObjectPoolChecker_IssueFields(t *testing.T) {
	src := `package p
import "bytes"
func f(items []string) {
	for _, s := range items {
		buf := bytes.NewBuffer(nil)
		buf.WriteString(s)
	}
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	c := &ObjectPoolChecker{}
	issues := c.Check(fset, f)
	if len(issues) != 1 {
		t.Fatalf("got %d issue(s), want 1", len(issues))
	}
	iss := issues[0]
	if iss.Checker != "ObjectPool" {
		t.Errorf("Checker = %q, want ObjectPool", iss.Checker)
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
	if !contains(iss.Suggestion, "sync.Pool") {
		t.Errorf("Suggestion %q should mention sync.Pool", iss.Suggestion)
	}
	if iss.Benchmark == "" {
		t.Error("Benchmark should be non-empty")
	}
}
