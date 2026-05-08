package benchmarks_test

import (
	"runtime"
	"sync"
	"testing"
)

// cpuWork simulates a small unit of CPU-bound work — enough to make goroutine
// spawn overhead visible without dominating the benchmark.
func cpuWork() int {
	sum := 0
	for i := 0; i < 100; i++ {
		sum += i
	}
	return sum
}

const taskCount = 200

func BenchmarkGoroutinePerTask(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		results := make([]int, taskCount)
		var wg sync.WaitGroup
		for j := 0; j < taskCount; j++ {
			wg.Add(1)
			idx := j
			go func() {
				defer wg.Done()
				results[idx] = cpuWork()
			}()
		}
		wg.Wait()
		_ = results
	}
}

func BenchmarkWorkerPool(b *testing.B) {
	b.ReportAllocs()
	numWorkers := runtime.NumCPU()
	for i := 0; i < b.N; i++ {
		results := make([]int, taskCount)
		jobs := make(chan int, taskCount)
		var wg sync.WaitGroup
		for w := 0; w < numWorkers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := range jobs {
					results[j] = cpuWork()
				}
			}()
		}
		for j := 0; j < taskCount; j++ {
			jobs <- j
		}
		close(jobs)
		wg.Wait()
		_ = results
	}
}
