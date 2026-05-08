package benchmarks_test

import (
	"sync"
	"testing"
)

type resource struct{ data []byte }

func newResource() *resource { return &resource{data: make([]byte, 4096)} }

// Eager: allocation happens at package init time regardless of whether it's used.
var eagerResource = newResource()

// Lazy: allocation deferred to first call; warm-path overhead is the Once check.
var lazyResource = sync.OnceValue(newResource)

// BenchmarkEagerAccess measures the cost of reading a package-level pointer —
// the baseline (no sync overhead).
func BenchmarkEagerAccess(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = eagerResource
	}
}

// BenchmarkLazyAccessWarm measures the warm-path cost of sync.OnceValue after
// the first call: a single atomic load.
func BenchmarkLazyAccessWarm(b *testing.B) {
	b.ReportAllocs()
	_ = lazyResource() // ensure initialized before timing
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = lazyResource()
	}
}
