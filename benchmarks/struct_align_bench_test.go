package benchmarks_test

import (
	"testing"
	"unsafe"
)

// misaligned: bool gaps between int64 fields force padding, bloating struct size.
type misaligned struct {
	a bool
	b int64
	c bool
	d int64
}

// aligned: largest fields first; booleans packed at the end, no padding waste.
type aligned struct {
	b int64
	d int64
	a bool
	c bool
}

func BenchmarkStructMisaligned(b *testing.B) {
	b.ReportAllocs()
	b.Logf("misaligned size: %d bytes", unsafe.Sizeof(misaligned{}))
	slice := make([]misaligned, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range slice {
			slice[j].a = j%2 == 0
			slice[j].b += int64(j)
			slice[j].c = j%3 == 0
			slice[j].d += int64(j)
		}
	}
}

func BenchmarkStructAligned(b *testing.B) {
	b.ReportAllocs()
	b.Logf("aligned size: %d bytes", unsafe.Sizeof(aligned{}))
	slice := make([]aligned, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range slice {
			slice[j].b += int64(j)
			slice[j].d += int64(j)
			slice[j].a = j%2 == 0
			slice[j].c = j%3 == 0
		}
	}
}
