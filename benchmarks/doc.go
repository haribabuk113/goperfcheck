// Package benchmarks contains microbenchmarks that measure the before/after
// performance impact of each goperfcheck rule. Run with:
//
//	go test -bench=. -benchmem ./benchmarks/
//
// Each pair of benchmarks shares the same workload and differs only in the
// pattern being flagged (the "No" / "Without" variant) vs. the recommended
// fix (the "With" / "Pool" / "Cached" variant). The numbers produced here
// are the source of truth for the Benchmark field in checker Issue structs.
package benchmarks
