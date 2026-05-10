# Checker Reference

goperfcheck ships 18 AST-based checkers grouped into three themes.
Run `goperfcheck -list-checkers` to see this list at any time, or
`goperfcheck -group <theme>` to run an entire theme at once.

---

## memory

| Checker | Severity | What it catches |
|---------|----------|-----------------|
| **MemPrealloc** | WARN/INFO | `append()` in loops without capacity; `make(map)` without a size hint |
| **ObjectPool** | WARN | High-churn allocations inside loops (bytes.Buffer, bufio.Writer, etc.) that could use `sync.Pool` |
| **StructAlign** | WARN | Struct fields ordered small → large, wasting padding bytes |
| **InterfaceBoxing** | INFO | `[]interface{}` params and empty-interface usage that causes heap boxing |
| **LazyInit** | INFO | Expensive `init()` functions and package-level allocations that could be deferred |
| **StackAlloc** | INFO | `new(primitiveType)` and `&localVar` returns that force heap allocation |
| **StringConcatLoop** | WARN | `s += expr` or `s = s + expr` inside a loop — O(n²) allocations; use `strings.Builder` |
| **RegexpCompile** | WARN | `regexp.Compile`/`MustCompile`/`CompilePOSIX`/`MustCompilePOSIX` inside a function body — compile once at package level |

## concurrency

| Checker | Severity | What it catches |
|---------|----------|-----------------|
| **GoroutinePool** | WARN | Unbounded goroutine creation inside loops — use a fixed worker pool |
| **ContextMisuse** | ERROR | `context.Context` stored in a struct field — contexts must be passed as parameters |
| **AtomicMutex** | INFO | Simple counters/flags guarded by a mutex — `sync/atomic` is faster |
| **TimeNowLoop** | INFO | `time.Now()` inside a loop — each call is a syscall; cache before the loop |
| **WaitGroupMisuse** | ERROR | `wg.Add(n)` called inside a goroutine literal — race: `Wait()` may return before the counter increments |
| **DeferInLoop** | WARN | `defer` inside a `for`/`range` loop — fires at function return, not loop-iteration end; allocates a closure per iteration |
| **SyncMapMisuse** | WARN | `sync.Map` where `map+sync.RWMutex` is faster — sync.Map boxes every key/value as `interface{}` and is only beneficial for append-only caches or per-goroutine disjoint key sets |

## io

| Checker | Severity | What it catches |
|---------|----------|-----------------|
| **ZeroCopy** | INFO | `append([]byte{}, src...)` — unnecessary buffer copy; use a slice reference for reads |
| **BufferedIO** | WARN | Unbuffered file writes in loops; missing `Flush()` on `bufio.Writer` |
| **Batching** | WARN | Individual DB/Redis/HTTP calls inside loops — collect and batch |
| **HTTPClientReuse** | WARN | `http.Client{…}` created inside a function — each client has its own transport pool, abandoning connection reuse |

---

## StructAlign — exported types and API safety

Reordering struct fields to eliminate padding is a safe, zero-risk change for
**unexported types** — only code in the same package can reference them.

For **exported types** the picture is different. Any caller that initialises the
struct with positional (unnamed) fields will fail to compile after a reorder:

```go
// caller in another package — positional literal
r := mypkg.Request{true, 42, "alice"}  // breaks if fields are reordered

// caller using named fields — safe regardless of field order
r := mypkg.Request{ok: true, id: 42, name: "alice"}
```

When goperfcheck detects padding waste in an exported struct it includes this
caveat in the suggestion:

```
💡 Exported type — audit callers for positional struct literals (Request{v1, v2, …})
   before reordering; reordering exported fields is a breaking API change for any
   caller that omits field names. If all call sites use named fields: reorder
   largest → smallest, int64/pointers first, then int32, int16, bool/byte last
```

**Before applying the fix to an exported struct**:

1. Search your own repo and any known dependents:
   ```bash
   grep -rn 'Request{[^}]*}' ./...   # look for positional literals
   ```
2. Check whether the package is consumed externally (published module). If so,
   treat a field reorder as a minor-version bump in a module that hasn't reached
   v1, or a major version bump after v1.
3. If all callers use named fields, the reorder is safe — apply it and the
   suggestion becomes a zero-risk memory win.

To suppress the finding on a type you have already audited:

```go
type Request struct { //goperfcheck:ignore StructAlign
    ok   bool
    id   int64
    name string
}
```

---

## MemPrealloc — capacity hint inference

MemPrealloc is the most sophisticated checker. It infers the best hint from the
surrounding AST rather than falling back to a fixed constant:

| Loop pattern | Suggested hint |
|---|---|
| `for _, v := range items { append(...) }` | `make([]T, 0, len(items))` |
| `for i := 0; i < n; i++ { append(...) }` | `make([]T, 0, n)` |
| `for i := 0; i <= n; i++ { append(...) }` | `make([]T, 0, n+1)` |
| `for _, v := range s.Items { append(...) }` | `make([]T, 0, len(s.Items))` |
| `m := make(map[K]V)` + range-populate loop | `make(map[K]V, len(items))` |
| No size derivable | `make([]T, 0, 8)` (conservative fallback) |

**Why the 8-slot fallback beats no hint at all**: omitting a capacity causes the
runtime to copy the backing array ≈log₂(finalLen) times as it doubles. Eight
slots eliminates the first three doublings (0→1→2→4→8) — the most expensive
ones relative to work done. For maps, Go rehashes at a load factor of ~6.5/8,
so even `make(map[K]V, 8)` avoids the first rehash entirely.

Nested loops are handled by stopping recursion at inner loops, so each
`append` is attributed to its **innermost** enclosing loop with the most
specific hint.

---

## SyncMapMisuse — when sync.Map is and isn't appropriate

`sync.Map` is optimised for exactly two access patterns:

| Pattern | sync.Map wins | Reason |
|---------|:---:|--------|
| Append-only cache (write once, read many) | ✓ | Read path is fully lock-free after initial Store |
| Per-goroutine disjoint key sets | ✓ | No contention between goroutines by construction |
| Growing registry (new keys added over time) | ✗ | Each Store on a new key grows the dirty map; promotion on next Load miss allocates |
| Mixed read/write on shared keys | ✗ | Dirty-to-read promotion adds overhead every write cycle |
| Single-goroutine or low-concurrency code | ✗ | No contention to amortise; boxing overhead dominates |

**Why the allocation cost matters**: `sync.Map` stores all keys and values as `interface{}`.
Every `Store` call boxes the key and value onto the heap — measured at **3 allocs/op**
compared to **0 allocs/op** for `map[K]V + sync.RWMutex`. At scale this
increases GC pause frequency and overall memory pressure.

```
sequential store+load (no contention):
  sync.Map:           662 ns/op   117 B/op   3 allocs/op
  map+sync.RWMutex:   308 ns/op    58 B/op   0 allocs/op  ← 2× faster, zero boxing
```

**When sync.Map genuinely wins** (high-contention read-only stable key set):
```
parallel reads, 8 goroutines, stable 64-key set:
  sync.Map:           4.4 ns/op   0 B/op   0 allocs/op   ← 9× faster
  map+sync.RWMutex:  39.7 ns/op   0 B/op   0 allocs/op
```

If you have this pattern, suppress the finding with `//goperfcheck:ignore SyncMapMisuse`.

**Patterns detected**:
- `sync.Map` or `*sync.Map` as a struct field
- `var m sync.Map` / `var m *sync.Map` (package-level or local)
- `m := sync.Map{}` (short variable declaration)

---

## Checker groups

Use `-group <name>` to run an entire theme. Names are case-insensitive.

```bash
goperfcheck -group memory        # 8 checkers
goperfcheck -group concurrency   # 7 checkers
goperfcheck -group io            # 4 checkers
```

`-group` and `-checker` are mutually exclusive. Combine with other flags:

```bash
goperfcheck -group concurrency -severity ERROR
goperfcheck -group memory -output memory.md
```

---

## Targeting a single checker

```bash
goperfcheck -checker mem-prealloc
goperfcheck -checker context-misuse -severity ERROR
```

Checker names are case-insensitive. Pass an unknown name and the tool prints all
valid names.

---

## How it works

goperfcheck uses Go's `go/ast` and `go/parser` packages to parse source files
and apply pattern-matching rules — no full type-checking (`go/types`) is needed.
This means it is fast and requires no build configuration, but it is
pattern-based and may produce false positives in unusual code shapes.

Each checker implements a single interface:

```go
type Checker interface {
    Name() string
    Check(fset *token.FileSet, file *ast.File) []Issue
}
```

Files are parsed in parallel worker goroutines (one `token.FileSet` per worker,
no shared mutable state). Results are deduplicated and sorted by file and line
before output.

---

## Limitations

- **Pattern-based, not type-checked**: detection uses AST patterns, not full
  type information, so some checks can produce false positives in unusual code
- **No dataflow analysis**: cannot prove a value escapes or track it across
  function calls
- **Heuristics**: some checks use naming conventions (e.g., receiver names for
  batching detection) that may not match every codebase style
