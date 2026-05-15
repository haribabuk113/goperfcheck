package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestSyncMapMisuseChecker(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "sync.Map struct field triggers",
			src: `package p
import "sync"
type Cache struct {
	m sync.Map
}`,
			wantN: 1,
		},
		{
			name: "*sync.Map struct field triggers",
			src: `package p
import "sync"
type Cache struct {
	m *sync.Map
}`,
			wantN: 1,
		},
		{
			name: "multiple sync.Map fields in one struct each trigger",
			src: `package p
import "sync"
type Cache struct {
	read  sync.Map
	write sync.Map
}`,
			wantN: 2,
		},
		{
			name: "var m sync.Map (package-level) triggers",
			src: `package p
import "sync"
var m sync.Map`,
			wantN: 1,
		},
		{
			name: "var m *sync.Map (package-level) triggers",
			src: `package p
import "sync"
var m *sync.Map`,
			wantN: 1,
		},
		{
			name: "var m sync.Map (local) triggers",
			src: `package p
import "sync"
func f() {
	var m sync.Map
	_ = m
}`,
			wantN: 1,
		},
		{
			name: "m := sync.Map{} short declaration triggers",
			src: `package p
import "sync"
func f() {
	m := sync.Map{}
	_ = m
}`,
			wantN: 1,
		},
		{
			name: "var m = sync.Map{} triggers",
			src: `package p
import "sync"
var m = sync.Map{}`,
			wantN: 1,
		},
		{
			name: "plain map does not trigger",
			src: `package p
func f() {
	m := make(map[string]int)
	_ = m
}`,
			wantN: 0,
		},
		{
			name: "sync.RWMutex field does not trigger",
			src: `package p
import "sync"
type S struct {
	mu sync.RWMutex
}`,
			wantN: 0,
		},
		{
			name: "sync.Mutex field does not trigger",
			src: `package p
import "sync"
type S struct {
	mu sync.Mutex
}`,
			wantN: 0,
		},
		{
			name: "other sync.Map usage via assignment does not trigger",
			src: `package p
import "sync"
func f(m *sync.Map) {
	m.Store("k", "v")
}`,
			wantN: 0,
		},
	}

	c := SyncMapMisuseChecker{}
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

func TestSyncMapMisuseChecker_IssueFields(t *testing.T) {
	src := `package p
import "sync"
type Cache struct {
	m sync.Map
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	c := SyncMapMisuseChecker{}
	issues := c.Check(fset, f)
	if len(issues) != 1 {
		t.Fatalf("got %d issue(s), want 1", len(issues))
	}
	iss := issues[0]
	if iss.Checker != "SyncMapMisuse" {
		t.Errorf("Checker = %q, want SyncMapMisuse", iss.Checker)
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
	if !contains(iss.Suggestion, "RWMutex") {
		t.Errorf("Suggestion %q should mention RWMutex", iss.Suggestion)
	}
	if iss.Benchmark == "" {
		t.Error("Benchmark should be non-empty")
	}
}
