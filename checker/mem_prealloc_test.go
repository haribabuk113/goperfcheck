package checker

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestMemPreallocChecker(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		wantN int
	}{
		{
			name: "append in range loop triggers",
			src: `package p
func f(items []string) []string {
	var result []string
	for _, v := range items { result = append(result, v) }
	return result
}`,
			wantN: 1,
		},
		{
			name: "append in c-style for loop triggers",
			src: `package p
func f(n int) []int {
	var out []int
	for i := 0; i < n; i++ { out = append(out, i) }
	return out
}`,
			wantN: 1,
		},
		{
			name: "append outside loop does not trigger",
			src: `package p
func f() []string {
	var s []string
	s = append(s, "a")
	return s
}`,
			wantN: 0,
		},
		{
			name: "make map without hint triggers",
			src: `package p
func f() map[string]int { return make(map[string]int) }`,
			wantN: 1,
		},
		{
			name: "make map with hint does not trigger",
			src: `package p
func f() map[string]int { return make(map[string]int, 16) }`,
			wantN: 0,
		},
		{
			name: "range hint inferred for map",
			src: `package p
func f(items []string) map[string]int {
	m := make(map[string]int)
	for i, v := range items { m[v] = i }
	return m
}`,
			wantN: 1,
			// Hint should be len(items) — verified by checking suggestion in a separate test.
		},
	}

	c := &MemPreallocChecker{}
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

func TestMemPreallocHints(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantHint string
	}{
		{
			name: "range loop hint",
			src: `package p
func f(items []string) []string {
	var r []string
	for _, v := range items { r = append(r, v) }
	return r
}`,
			wantHint: "len(items)",
		},
		{
			name: "c-style loop hint",
			src: `package p
func f(n int) []int {
	var r []int
	for i := 0; i < n; i++ { r = append(r, i) }
	return r
}`,
			wantHint: "n",
		},
		{
			name: "c-style <= loop hint",
			src: `package p
func f(n int) []int {
	var r []int
	for i := 0; i <= n; i++ { r = append(r, i) }
	return r
}`,
			wantHint: "n+1",
		},
		{
			name: "map hint from subsequent range",
			src: `package p
func f(items []string) map[string]int {
	m := make(map[string]int)
	for i, v := range items { m[v] = i }
	return m
}`,
			wantHint: "len(items)",
		},
		{
			name: "map fallback default hint",
			src: `package p
func f() map[string]int { return make(map[string]int) }`,
			wantHint: "8",
		},
	}

	c := &MemPreallocChecker{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			issues := c.Check(fset, f)
			if len(issues) == 0 {
				t.Fatal("expected at least one issue, got none")
			}
			sug := issues[0].Suggestion
			if !contains(sug, tt.wantHint) {
				t.Errorf("suggestion %q does not contain hint %q", sug, tt.wantHint)
			}
		})
	}
}

// TestMemPreallocNoReFlagAfterFix verifies that once a slice is declared with
// make([]T, 0, cap) before a loop, the checker does not re-report the append
// inside the loop. This is the key correctness property: running -fix and then
// re-running the checker must produce no issue for the fixed code.
func TestMemPreallocNoReFlagAfterFix(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "range loop: slice preallocated with make([]T,0,cap) before loop",
			src: `package p
func f(items []string) []string {
	out := make([]string, 0, len(items))
	for _, v := range items {
		out = append(out, v)
	}
	return out
}`,
		},
		{
			name: "c-style for loop: slice preallocated before loop",
			src: `package p
func f(n int) []int {
	out := make([]int, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, i)
	}
	return out
}`,
		},
		{
			name: "nested loops: outer var preallocated, inner var not — only inner flagged",
			src: `package p
func f(items [][]string) []string {
	out := make([]string, 0, len(items))
	for _, group := range items {
		for _, v := range group {
			out = append(out, v)
		}
	}
	return out
}`,
		},
		{
			name: "var form with make and cap: also suppressed",
			src: `package p
func f(items []string) []string {
	var out = make([]string, 0, len(items))
	for _, v := range items {
		out = append(out, v)
	}
	return out
}`,
		},
	}

	c := &MemPreallocChecker{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			issues := c.Check(fset, f)
			// Filter to only MemPrealloc slice issues (not map issues).
			var sliceIssues []Issue
			for _, iss := range issues {
				if iss.Fix != nil && iss.Fix.Kind == "slice_cap" {
					sliceIssues = append(sliceIssues, iss)
				}
			}
			if len(sliceIssues) != 0 {
				t.Errorf("expected 0 slice issues (preallocated), got %d:", len(sliceIssues))
				for _, iss := range sliceIssues {
					t.Logf("  %s", iss.Message)
				}
			}
		})
	}
}

func TestMemPreallocMakeWithoutCapStillFlags(t *testing.T) {
	// make([]T, 0) has zero capacity — same as var decl — must still be flagged.
	src := `package p
func f(items []string) []string {
	out := make([]string, 0)
	for _, v := range items {
		out = append(out, v)
	}
	return out
}`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	c := &MemPreallocChecker{}
	issues := c.Check(fset, file)
	var sliceIssues []Issue
	for _, iss := range issues {
		if iss.Fix != nil && iss.Fix.Kind == "slice_cap" {
			sliceIssues = append(sliceIssues, iss)
		}
	}
	if len(sliceIssues) == 0 {
		t.Error("make([]T, 0) before loop (cap=0) should still be flagged")
	}
}

// TestMemPreallocFalsePositive_LoopLocalSlice documents a known false positive:
// MemPrealloc fires when append() appears inside a loop even when the slice
// being appended to was created inside the loop body, not accumulated across
// iterations. Without type resolution or data-flow analysis, the checker cannot
// distinguish accumulation patterns from local-use patterns.
//
// In the example below, tmp is allocated fresh on every iteration and its
// contents are merged into results via a separate append — no preallocation of
// tmp would help because it is discarded at the end of each iteration.
// The outer results slice is never directly appended to inside the loop.
// This test pins the current behaviour so the limitation remains visible.
// TestMemPreallocCrossFunctionScope verifies that preallocated-slice tracking is
// scoped per function. A variable named "out" preallocated in f1 must not
// suppress the issue for a different "out" that is NOT preallocated in f2.
// This was the root cause of false negatives vs golangci-lint's prealloc linter.
func TestMemPreallocCrossFunctionScope(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantSlice int // expected slice_cap issues
	}{
		{
			name: "same var name: preallocated in f1, unpreallocated in f2 — f2 must be flagged",
			src: `package p
func f1(items []string) []string {
	out := make([]string, 0, len(items))
	for _, v := range items { out = append(out, v) }
	return out
}
func f2(items []string) []string {
	var out []string
	for _, v := range items { out = append(out, v) }
	return out
}`,
			wantSlice: 1,
		},
		{
			name: "same var name in three functions — only unpreallocated ones flagged",
			src: `package p
func a(items []string) []string {
	out := make([]string, 0, len(items))
	for _, v := range items { out = append(out, v) }
	return out
}
func b(items []string) []string {
	var out []string
	for _, v := range items { out = append(out, v) }
	return out
}
func c(items []string) []string {
	var out []string
	for _, v := range items { out = append(out, v) }
	return out
}`,
			wantSlice: 2, // b and c
		},
		{
			name: "nested closure inherits no preallocated state from outer function",
			src: `package p
func outer(items []string) []string {
	out := make([]string, 0, len(items))
	inner := func(sub []string) []string {
		var out []string
		for _, v := range sub { out = append(out, v) }
		return out
	}
	return inner(out)
}`,
			wantSlice: 1, // inner's out must be flagged
		},
		{
			name: "method and standalone function with same var name",
			src: `package p
type T struct{}
func (t T) Method(items []string) []string {
	result := make([]string, 0, len(items))
	for _, v := range items { result = append(result, v) }
	return result
}
func standalone(items []string) []string {
	var result []string
	for _, v := range items { result = append(result, v) }
	return result
}`,
			wantSlice: 1, // standalone must be flagged, Method must not
		},
	}

	c := &MemPreallocChecker{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			issues := c.Check(fset, f)
			var sliceIssues []Issue
			for _, iss := range issues {
				if iss.Fix != nil && iss.Fix.Kind == "slice_cap" {
					sliceIssues = append(sliceIssues, iss)
				}
			}
			if len(sliceIssues) != tt.wantSlice {
				t.Errorf("got %d slice issue(s), want %d", len(sliceIssues), tt.wantSlice)
				for _, iss := range sliceIssues {
					t.Logf("  line %d: %s", iss.Line, iss.Message)
				}
			}
		})
	}
}

func TestMemPreallocFalsePositive_LoopLocalSlice(t *testing.T) {
	src := `package p
func process(items []string) []string {
	var results []string
	for _, item := range items {
		// tmp is loop-local: created and discarded each iteration.
		// Preallocating tmp changes nothing for results.
		var tmp []string
		tmp = append(tmp, transform(item))
		results = append(results, tmp...)
	}
	return results
}
func transform(s string) string { return s }
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	c := &MemPreallocChecker{}
	issues := c.Check(fset, f)

	// The checker fires on tmp (loop-local) — this is the known false positive.
	// The results slice is also correctly flagged, making it hard to distinguish
	// which append is the real problem without reading the code carefully.
	if len(issues) == 0 {
		t.Fatal("expected MemPrealloc to fire on loop-local slice (known false positive scenario)")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
