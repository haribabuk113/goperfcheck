package benchmarks_test

import "testing"

const benchN = 1000

// Slice — append without vs. with capacity hint

func BenchmarkSliceNoPrealloc(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var s []int
		for j := 0; j < benchN; j++ {
			s = append(s, j)
		}
		_ = s
	}
}

func BenchmarkSlicePrealloc(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s := make([]int, 0, benchN)
		for j := 0; j < benchN; j++ {
			s = append(s, j)
		}
		_ = s
	}
}

// Map — make without vs. with size hint

func BenchmarkMapNoHint(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m := make(map[int]int)
		for j := 0; j < benchN; j++ {
			m[j] = j
		}
		_ = m
	}
}

func BenchmarkMapWithHint(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m := make(map[int]int, benchN)
		for j := 0; j < benchN; j++ {
			m[j] = j
		}
		_ = m
	}
}
