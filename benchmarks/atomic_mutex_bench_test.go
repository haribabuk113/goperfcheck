package benchmarks_test

import (
	"sync"
	"sync/atomic"
	"testing"
)

// Both benchmarks use RunParallel to simulate real contention across goroutines.

func BenchmarkMutexCounter(b *testing.B) {
	b.ReportAllocs()
	var mu sync.Mutex
	var counter int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			counter++
			mu.Unlock()
		}
	})
	_ = counter
}

func BenchmarkAtomicCounter(b *testing.B) {
	b.ReportAllocs()
	var counter atomic.Int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			counter.Add(1)
		}
	})
	_ = counter.Load()
}
