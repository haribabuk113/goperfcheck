# goperfcheck - Go Performance Static Analyzer

A comprehensive static analysis tool that checks Go code against all performance best practices from **https://goperf.dev**.

## Overview

`goperfcheck` scans Go source files and reports violations of 12+ performance optimization patterns covered in the goperf.dev guide. It runs without external dependencies (only uses Go's standard library AST/parser).

## Features

The tool checks for violations across these categories:

### 1. **Memory Management**
- **MemPrealloc**: Detects `append()` in loops without preallocated capacity and `make(map)` calls without size hints
- **ObjectPool**: Flags high-churn allocations (bytes.Buffer, bufio.Writer, etc.) in loops that could use `sync.Pool`
- **StructAlign**: Detects misaligned struct fields (small before large) that waste padding memory

### 2. **Concurrency & Synchronization**
- **GoroutinePool**: Flags unbounded goroutine creation in loops (should use worker pools)
- **AtomicMutex**: Suggests `sync/atomic` operations for simple counters/flags (~27% faster than mutexes)
- **ContextMisuse**: Detects `context.Context` stored in struct fields (critical error - contexts must be passed as parameters)

### 3. **I/O Optimization**
- **BufferedIO**: Detects unbuffered file writes in loops and missing `Flush()` calls on `bufio.Writer`
- **ZeroCopy**: Flags unnecessary buffer copies (append([]byte{}, src...), copy in loops)
- **Batching**: Detects individual DB/Redis/HTTP operations in loops that should be batched

### 4. **Initialization & Allocation**
- **LazyInit**: Flags expensive init() functions and package-level allocations that could be deferred
- **StackAlloc**: Detects `new(primitiveType)` and `&localVar` returns that force heap allocation
- **InterfaceBoxing**: Detects `[]interface{}` and empty interface parameters that cause heap boxing

## Installation

The tool is built as a Go module command. Build it with:

```bash
cd cmd/goperfcheck
go build -o goperfcheck
```

Or from the repo root:

```bash
go build -o goperfcheck ./cmd/goperfcheck
```

## Usage

### Basic scan of current directory:
```bash
./goperfcheck
```

### Scan a specific directory:
```bash
./goperfcheck -dir ./rma
./goperfcheck -dir /path/to/code
```

### Filter by severity (INFO, WARN, ERROR):
```bash
./goperfcheck -severity WARN        # Show warnings and errors only
./goperfcheck -severity ERROR       # Show critical errors only
```

### Skip tests:
```bash
./goperfcheck -skip-tests
```

### Full option list:
```bash
./goperfcheck -help
```

## Output Format

Issues are grouped by file and include:

- **File path** relative to the scanned directory
- **Severity level** (ERROR/WARN/INFO)
- **Checker name** (e.g., MemPrealloc, GoroutinePool)
- **Location** (file:line:column)
- **Message** explaining the issue
- **Suggestion** for how to fix it

Example:
```
📁 rma/aggregation/aggregation.go
   [WARN] StructAlign:64:2
   ⚠  struct "AggregationsWork": field "RepairMode" (~1B) before "Emitter" (~8B) — misalignment causes padding waste
   💡 Reorder fields largest → smallest: int64/pointers first, then int32, int16, bool/byte last

   [ERROR] ContextMisuse:87:2
   ⚠  context.Context stored in struct field "CTX" — contexts must never be stored in structs
   💡 Pass context.Context as the first parameter to every function that needs it
```

## Real-World Results

When run on a typical Go project, the tool can identify hundreds of optimization opportunities:

| Issue Category | Example Count |
|---|---|
| Memory Preallocation | ~50-100 |
| Goroutine Pools | ~30-80 |
| Interface Boxing | ~20-50 |
| Struct Alignment | ~5-20 |
| Context Misuse | ~1-5 |
| Buffered I/O | ~2-10 |
| **Total** | **100s-1000s** |

## Performance Impact

Addressing all issues found by goperfcheck can yield:

- **Memory**: 5-10% reduction in struct sizes (through alignment fixes)
- **GC**: 20-40% reduction in garbage collection pressure (via pooling)
- **Throughput**: 2-12x improvement for I/O operations (via batching/buffering)
- **Latency**: Elimination of unbounded goroutine creation risks

## How It Works

goperfcheck uses Go's AST (Abstract Syntax Tree) analysis to:

1. Walk all `.go` files in the target directory (excluding vendor/ and test files)
2. Parse each file without requiring full type information
3. Apply 12 specialized checkers that look for anti-patterns
4. Report violations with line numbers and fix suggestions

No external dependencies - works with only Go's standard library.

## Limitations

- **Pattern-based**: Detection is based on AST patterns, not full type checking, so can have false positives
- **Heuristics**: Some checks use heuristics (e.g., receiver variable names for batching detection)
- **No dataflow**: Cannot track data flow across functions (e.g., proving a value doesn't escape)

## For More Information

See **https://goperf.dev** for in-depth explanations of each rule, benchmarks, and code examples.
