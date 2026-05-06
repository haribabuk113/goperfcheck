# goperfcheck - Quick Start Guide

## TL;DR

A **static analyzer** that checks your Go code against all 15+ performance patterns from **https://goperf.dev**.

## Building

```bash
cd cmd/goperfcheck
go build -o goperfcheck
# Now you have ./goperfcheck binary
```

## Running

```bash
# Scan current repo for all issues
./goperfcheck

# Scan specific directory, show only warnings and errors
./goperfcheck -dir ./rma -severity WARN

# Scan, hiding INFO-level messages
./goperfcheck -severity WARN

# Show only CRITICAL errors
./goperfcheck -severity ERROR

# Skip test files
./goperfcheck -skip-tests
```

## What It Checks

| Pattern | Severity | Impact |
|---------|----------|--------|
| **append() in loop without capacity** | ⚠ WARN | 4x slower, 19x more allocations |
| **Context stored in struct** | ❌ ERROR | Broken cancellation, subtle bugs |
| **go in loop without pool** | ⚠ WARN | Unbounded goroutines, crashes |
| **Struct field misalignment** | ⚠ WARN | Memory waste |
| **Interface{} boxing** | ⓘ INFO | Heap pressure |
| **Unbuffered I/O in loop** | ⚠ WARN | 12x slower (can be 10K+ syscalls) |
| **Missing Flush() on bufio** | ❌ ERROR | Data loss |
| **Individual DB ops in loop** | ⚠ WARN | 2-12x slower |
| **new(int)** etc | ⚠ WARN | Unnecessary heap allocation |
| ... 10+ more patterns ... | | |

## Example Output

```
📁 redis/redisClient.go
   [ERROR] ContextMisuse:45:2
   ⚠  context.Context stored in struct field "ctx" — contexts must never be stored in structs
   💡 Pass context.Context as the first parameter to every function that needs it

📁 rma/aggregation/aggregation.go
   [WARN] StructAlign:64:2
   ⚠  struct "AggregationsWork": field "RepairMode" (~1B) before "Emitter" (~8B) — misalignment causes padding waste
   💡 Reorder fields largest → smallest: int64/pointers first, then int32, int16, bool/byte last

   [WARN] GoroutinePool:419:3
   ⚠  goroutine spawned inside a loop without a worker pool — unbounded concurrency risks saturation
   💡 Create a fixed pool of N goroutines that read from a buffered job channel; set N ≈ runtime.NumCPU()

   [WARN] MemPrealloc:1031:13
   ⚠  append() inside a loop — repeated reallocations occur when the backing array runs out of capacity
   💡 Before the loop use make([]T, 0, expectedLen) or make([]T, n) with index assignment

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Found 1300 performance issue(s)
```

## Fix Priority

1. **Fix ERROR issues first** (data loss, broken semantics)
   - Context stored in structs
   - Missing Flush() calls

2. **Fix WARN issues by frequency** (performance)
   - MemPrealloc: Most common, easy fix
   - GoroutinePool: High impact
   - StructAlign: Low effort, memory savings

3. **Review INFO issues** (polish)
   - Interface boxing: Consider when in hot paths

## Example Fixes

### ❌ Before: append() in loop
```go
var result []Item
for _, item := range items {
    result = append(result, item)  // Reallocates repeatedly!
}
return result
```

### ✅ After: Preallocated
```go
result := make([]Item, 0, len(items))  // Pre-allocate capacity
for _, item := range items {
    result = append(result, item)  // No reallocations
}
return result
```

---

### ❌ Before: Context in struct
```go
type Service struct {
    ctx context.Context  // WRONG!
}
```

### ✅ After: Pass as parameter
```go
type Service struct {
    // Remove ctx field
}

func (s *Service) DoWork(ctx context.Context) error {
    // ctx passed explicitly
}
```

---

### ❌ Before: Unbounded goroutines
```go
for _, item := range items {
    go process(item)  // Unbounded concurrency!
}
```

### ✅ After: Worker pool
```go
jobs := make(chan Item, 100)
for i := 0; i < runtime.NumCPU(); i++ {
    go worker(jobs)  // Fixed pool size
}
for _, item := range items {
    jobs <- item
}
close(jobs)
```

---

### ❌ Before: Struct misalignment
```go
type Record struct {
    enabled bool    // 1 byte - wastes 7 bytes padding!
    id      int64   // 8 bytes
    count   int64   // 8 bytes
}
```

### ✅ After: Aligned
```go
type Record struct {
    id      int64   // 8 bytes first
    count   int64   // 8 bytes
    enabled bool    // 1 byte last
}
```

## Performance Impact

| Fix Type | Speedup | Typical Count |
|----------|---------|-------|
| Preallocation | 4x | 50-200 |
| Worker pools | 2-5x | 30-100 |
| Struct alignment | 5-10% memory | 5-20 |
| Buffered I/O | 12x | 5-50 |
| Batching | 2-12x | 20-50 |

Running all recommended fixes can yield **20-40% GC reduction** and **10-100x throughput for I/O**.

## Reference

- **Website**: https://goperf.dev
- **GitHub project**: This tool is located at `/cmd/goperfcheck`
- **Checker source**: `/cmd/goperfcheck/checker/`

## See Also

- See `GOPERFCHECK_SUMMARY.md` for detailed implementation info
- See `cmd/goperfcheck/README.md` for full documentation
- See `cmd/goperfcheck/checker/` for individual checker code
