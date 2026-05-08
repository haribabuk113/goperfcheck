package benchmarks_test

import "testing"

var payload = make([]byte, 4096)

func BenchmarkCopyAppend(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	for i := 0; i < b.N; i++ {
		dst := append([]byte{}, payload...)
		_ = dst
	}
}

func BenchmarkSliceRef(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	for i := 0; i < b.N; i++ {
		dst := payload[:]
		_ = dst
	}
}
