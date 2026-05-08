package benchmarks_test

import (
	"bytes"
	"sync"
	"testing"
)

var bufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

func BenchmarkBufferNoPool(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := new(bytes.Buffer)
		buf.WriteString("hello world benchmark data")
		_ = buf.String()
	}
}

func BenchmarkBufferWithPool(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := bufPool.Get().(*bytes.Buffer)
		buf.Reset()
		buf.WriteString("hello world benchmark data")
		_ = buf.String()
		bufPool.Put(buf)
	}
}
