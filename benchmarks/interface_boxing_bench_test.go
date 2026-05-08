package benchmarks_test

import "testing"

var intSrc = func() []int {
	s := make([]int, 100)
	for i := range s {
		s[i] = i
	}
	return s
}()

func BenchmarkInterfaceSlice(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s := make([]interface{}, 0, len(intSrc))
		for _, v := range intSrc {
			s = append(s, v)
		}
		_ = s
	}
}

func BenchmarkTypedSlice(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s := make([]int, 0, len(intSrc))
		for _, v := range intSrc {
			s = append(s, v)
		}
		_ = s
	}
}
