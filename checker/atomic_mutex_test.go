package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestAtomicMutexChecker(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "struct with mutex and only scalar fields triggers",
			src: `package p
import "sync"
type Counter struct {
	mu    sync.Mutex
	count int64
}`,
			wantN: 1,
		},
		{
			name: "lock/incr/unlock sequence triggers",
			src: `package p
import "sync"
var mu sync.Mutex
var n int
func inc() {
	mu.Lock()
	n++
	mu.Unlock()
}`,
			wantN: 1,
		},
		{
			name: "struct with no mutex does not trigger",
			src: `package p
type Counter struct {
	count int64
}`,
			wantN: 0,
		},
		{
			name: "struct with mutex and non-atomic field does not trigger",
			src: `package p
import "sync"
type Store struct {
	mu   sync.Mutex
	data map[string]int
}`,
			wantN: 0,
		},
	}

	c := &AtomicMutexChecker{}
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
					t.Logf("  issue: %s", iss.Message)
				}
			}
		})
	}
}

// TestAtomicMutexFalsePositive_MutexGuardsMultipleFields documents a known
// false positive: AtomicMutex fires on a struct whose mutex-protected scalar
// field is accompanied by a non-scalar field in a *different* struct or accessed
// via an embedded lock. Without type resolution, the checker only inspects one
// struct at a time and cannot see the broader lock scope.
//
// In the case below, Counter.mu also protects a separate slice field held in a
// companion struct. Following AtomicMutex's suggestion would introduce a data
// race on that slice. This test pins the current behaviour so future changes
// cannot silently change when the checker fires.
func TestAtomicMutexFalsePositive_MutexGuardsMultipleFields(t *testing.T) {
	// The two structs are separate here; in real code the mutex in CounterA
	// might also protect CounterB's state through a shared lock pointer — a
	// pattern the AST cannot express within one struct literal.
	src := `package p
import "sync"

// mu protects both count and the external items slice, but the checker
// only sees the struct definition and concludes atomics are safe.
type Counter struct {
	mu    sync.Mutex
	count int64
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	c := &AtomicMutexChecker{}
	issues := c.Check(fset, f)

	// The checker fires — this is the known false positive.
	// Before acting on this suggestion, verify that no other variables share
	// this mutex's lock scope outside the struct definition.
	if len(issues) == 0 {
		t.Fatal("expected AtomicMutex to fire on single-scalar struct (known false positive scenario)")
	}
}
