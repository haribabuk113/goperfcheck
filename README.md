# goperfcheck — Go Performance Static Analyzer

<p align="center">
  <img src="assets/mascot.png" alt="goperfcheck mascot" width="200"/>
</p>

[![CI](https://github.com/haribabuk113/goperfcheck/actions/workflows/ci.yml/badge.svg)](https://github.com/haribabuk113/goperfcheck/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/haribabuk113/goperfcheck.svg)](https://pkg.go.dev/github.com/haribabuk113/goperfcheck)
[![Go Report Card](https://goreportcard.com/badge/github.com/haribabuk113/goperfcheck)](https://goreportcard.com/report/github.com/haribabuk113/goperfcheck)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A static analysis tool that checks Go code against all performance best practices from **https://goperf.dev**.

## Overview

`goperfcheck` scans Go source files and reports violations of 12+ performance optimization patterns covered in the goperf.dev guide. It runs without external dependencies (only uses Go's standard library AST/parser).

## Features

The tool checks for violations across these categories:

### 1. **Memory Management**
- **MemPrealloc**: Detects `append()` in loops without preallocated capacity and `make(map)` calls without size hints.
  The checker infers the best capacity hint directly from the surrounding code:
  - `for _, v := range items` → suggests `make([]T, 0, len(items))`
  - `for i := 0; i < n; i++` → suggests `make([]T, 0, n)`
  - `for i := 0; i <= n; i++` → suggests `make([]T, 0, n+1)`
  - `m := make(map[K]V)` followed by `for _, v := range items { m[...] = ... }` → suggests `make(map[K]V, len(items))`
  - When no size can be inferred, it falls back to a conservative default of **8**

  **Why default 8 beats no hint**: Omitting a capacity causes the runtime to copy the backing array ≈log₂(finalLen) times as it doubles. Preallocating 8 slots eliminates the first three doublings (cap 0→1→2→4→8) — the most expensive ones relative to the work done. For maps, Go rehashes at a load-factor of ~6.5/8, so even `make(map[K]V, 8)` avoids the first rehash entirely. The memory cost (8×sizeof(element)) is negligible because those bytes would be allocated anyway once the data structure grows.
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

**Install directly with Go (recommended):**
```bash
go install github.com/haribabuk113/goperfcheck/cmd/goperfcheck@latest
```

**Build from source:**
```bash
git clone https://github.com/haribabuk113/goperfcheck.git
cd goperfcheck
make build        # produces ./goperfcheck
# or: go build -o goperfcheck ./cmd/goperfcheck
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

### Check a single file:
```bash
./goperfcheck -file ./pkg/handler.go
```

### Run only one checker:
```bash
./goperfcheck -checker mem-prealloc
./goperfcheck -checker context-misuse -severity ERROR
```
Pass an unknown name and the tool prints all valid checker names.

### Check only files staged for the next git commit:
```bash
./goperfcheck -git-staged
./goperfcheck -git-staged -severity WARN
```
This is useful as a pre-commit hook: it runs the checker only on the diff you are about to commit rather than the entire repository, keeping feedback fast.

### Emit JSON for editor integrations:
```bash
./goperfcheck -format json
./goperfcheck -file ./pkg/cache.go -format json
```
Output is a JSON array of issue objects. Fields: `checker`, `file`, `line`,
`column`, `severity`, `message`, and optionally `rule` and `suggestion`.

### Write a Markdown report:
```bash
./goperfcheck -output report.md
./goperfcheck -dir ./myproject -output report.md -severity WARN
```
Issues are grouped by category with a summary table and per-category detail sections.

### Write a SARIF report for GitHub Code Scanning:
```bash
./goperfcheck -dir . -output results.sarif
```
When `-output` ends in `.sarif`, the report is written as SARIF 2.1.0 instead of
Markdown. Upload it to GitHub to get inline annotations on pull requests:

```yaml
# .github/workflows/perf.yml
- name: Run goperfcheck
  run: goperfcheck -dir . -output results.sarif
  continue-on-error: true

- name: Upload to GitHub Code Scanning
  uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: results.sarif
```

### Print version:
```bash
./goperfcheck -version
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
