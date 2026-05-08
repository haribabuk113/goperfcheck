package benchmarks_test

import (
	"testing"
	"time"
)

const loopIter = 1000

func BenchmarkTimeNowInLoop(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var last time.Time
		for j := 0; j < loopIter; j++ {
			last = time.Now()
		}
		_ = last
	}
}

func BenchmarkTimeNowCachedBeforeLoop(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		t := time.Now()
		var last time.Time
		for j := 0; j < loopIter; j++ {
			last = t
		}
		_ = last
	}
}
