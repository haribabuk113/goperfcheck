package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestWaitGroupMisuseChecker(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "wg.Add(1) inside goroutine triggers",
			src: `package p
import "sync"
func f(items []string) {
	var wg sync.WaitGroup
	for _, item := range items {
		go func(s string) {
			wg.Add(1)
			defer wg.Done()
			_ = s
		}(item)
	}
	wg.Wait()
}`,
			wantN: 1,
		},
		{
			name: "wg.Add(1) before goroutine does not trigger",
			src: `package p
import "sync"
func f(items []string) {
	var wg sync.WaitGroup
	for _, item := range items {
		wg.Add(1)
		go func(s string) {
			defer wg.Done()
			_ = s
		}(item)
	}
	wg.Wait()
}`,
			wantN: 0,
		},
		{
			name: "wg.Add(n) with non-1 literal inside goroutine triggers",
			src: `package p
import "sync"
func f() {
	var wg sync.WaitGroup
	go func() {
		wg.Add(5)
		wg.Done()
	}()
	wg.Wait()
}`,
			wantN: 1,
		},
		{
			name: "multiple goroutines each with wg.Add inside each trigger",
			src: `package p
import "sync"
func f() {
	var wg sync.WaitGroup
	go func() {
		wg.Add(1)
		wg.Done()
	}()
	go func() {
		wg.Add(1)
		wg.Done()
	}()
	wg.Wait()
}`,
			wantN: 2,
		},
		{
			name: "chained selector s.wg.Add inside goroutine does not trigger",
			src: `package p
type S struct{ wg interface{ Add(int) } }
func f(s S) {
	go func() {
		s.wg.Add(1)
	}()
}`,
			wantN: 0,
		},
		{
			name: "wg.Add with non-literal argument inside goroutine does not trigger",
			src: `package p
import "sync"
func f(n int) {
	var wg sync.WaitGroup
	go func() {
		wg.Add(n)
		wg.Done()
	}()
	wg.Wait()
}`,
			wantN: 0,
		},
		{
			name: "wg.Add() with no arguments inside goroutine does not trigger",
			src: `package p
import "sync"
func f() {
	var wg sync.WaitGroup
	go func() {
		wg.Add()
	}()
}`,
			wantN: 0,
		},
		{
			name: "other .Add(1) method (not wg) inside goroutine triggers (heuristic)",
			src: `package p
func f(list List) {
	go func() {
		list.Add(1)
	}()
}`,
			// The checker is heuristic: any ident.Add(intLit) inside a goroutine fires.
			wantN: 1,
		},
		{
			name: "goroutine with no Add call does not trigger",
			src: `package p
func f() {
	go func() {
		doWork()
	}()
}`,
			wantN: 0,
		},
		{
			name: "go statement with non-literal func does not trigger",
			src: `package p
func f(fn func()) {
	go fn()
}`,
			wantN: 0,
		},
	}

	c := WaitGroupMisuseChecker{}
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

func TestWaitGroupMisuseChecker_IssueFields(t *testing.T) {
	src := `package p
import "sync"
func f() {
	var wg sync.WaitGroup
	go func() {
		wg.Add(1)
		defer wg.Done()
		doWork()
	}()
	wg.Wait()
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	c := WaitGroupMisuseChecker{}
	issues := c.Check(fset, f)
	if len(issues) != 1 {
		t.Fatalf("got %d issue(s), want 1", len(issues))
	}
	iss := issues[0]
	if iss.Checker != "WaitGroupMisuse" {
		t.Errorf("Checker = %q, want WaitGroupMisuse", iss.Checker)
	}
	if iss.Severity != SeverityError {
		t.Errorf("Severity = %v, want Error", iss.Severity)
	}
	if iss.Line == 0 {
		t.Error("Line should be non-zero")
	}
	if !contains(iss.Rule, "goperf.dev") {
		t.Errorf("Rule %q missing goperf.dev link", iss.Rule)
	}
	if !contains(iss.Message, "race") {
		t.Errorf("Message %q should mention race", iss.Message)
	}
	if iss.Benchmark == "" {
		t.Error("Benchmark should be non-empty")
	}
}
