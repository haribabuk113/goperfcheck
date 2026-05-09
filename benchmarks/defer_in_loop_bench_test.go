package benchmarks

import (
	"os"
	"testing"
)

// withDeferInLoop creates N temp files in a loop using defer inside the loop.
// Each iteration allocates a closure and the files are closed at function return.
func withDeferInLoop(n int) {
	for i := 0; i < n; i++ {
		f, _ := os.CreateTemp("", "bench")
		defer f.Close() //nolint:staticcheck,errcheck // intentional pattern under test
	}
}

// withImmediateClose creates N temp files, closing each one immediately via an
// inline IIFE — the idiomatic replacement for defer-in-loop.
func withImmediateClose(n int) {
	for i := 0; i < n; i++ {
		func() {
			f, _ := os.CreateTemp("", "bench")
			defer f.Close() //nolint:errcheck // benchmark: close error not meaningful
		}()
	}
}

func BenchmarkDeferInLoop(b *testing.B) {
	for b.Loop() {
		withDeferInLoop(10)
	}
}

func BenchmarkImmediateClose(b *testing.B) {
	for b.Loop() {
		withImmediateClose(10)
	}
}
