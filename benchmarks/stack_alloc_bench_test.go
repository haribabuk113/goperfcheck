package benchmarks_test

import "testing"

// sink prevents the compiler from eliminating the allocation entirely.
var sink *int
var sinkVal int

func BenchmarkHeapAllocNew(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p := new(int)
		*p = i
		sink = p // escape to heap — mirrors real use of new()
	}
}

func BenchmarkStackAllocVar(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		x := i
		sinkVal = x // keep on stack — no pointer taken
	}
}
