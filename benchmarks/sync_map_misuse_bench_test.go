package benchmarks_test

import (
	"sync"
	"testing"
)

// BenchmarkSyncMapSequential and BenchmarkRWMutexMapSequential measure the
// common misuse pattern: a goroutine-safe map for sequential or low-concurrency
// code where the type is chosen "because the name sounds right". sync.Map boxes
// every key and value as interface{}, paying 3 heap allocations per Store even
// when there is no contention at all.
func BenchmarkSyncMapSequential(b *testing.B) {
	b.ReportAllocs()
	var m sync.Map
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Store(i, i)
		m.Load(i)
	}
}

func BenchmarkRWMutexMapSequential(b *testing.B) {
	b.ReportAllocs()
	var mu sync.RWMutex
	m := make(map[int]int)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mu.Lock()
		m[i] = i
		mu.Unlock()
		mu.RLock()
		_ = m[i]
		mu.RUnlock()
	}
}

// BenchmarkSyncMapHighContention and BenchmarkRWMutexMapHighContention show
// the narrow scenario where sync.Map is genuinely appropriate: a stable, small
// key space (written once at startup, read many times after) under heavy
// concurrent load. Here sync.Map's lock-free read path beats RWMutex.
const stableKeys = 64

func BenchmarkSyncMapHighContention(b *testing.B) {
	b.ReportAllocs()
	var m sync.Map
	for i := 0; i < stableKeys; i++ {
		m.Store(i, i) // written once
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			m.Load(i % stableKeys) // read many times
			i++
		}
	})
}

func BenchmarkRWMutexMapHighContention(b *testing.B) {
	b.ReportAllocs()
	var mu sync.RWMutex
	m := make(map[int]int, stableKeys)
	for i := 0; i < stableKeys; i++ {
		m[i] = i
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			mu.RLock()
			_ = m[i%stableKeys]
			mu.RUnlock()
			i++
		}
	})
}
