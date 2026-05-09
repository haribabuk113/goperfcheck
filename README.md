# goperfcheck — Go Performance Static Analyzer

<p align="center">
  <img src="assets/mascot.png" alt="goperfcheck mascot" width="200"/>
</p>

[![CI](https://github.com/haribabuk113/goperfcheck/actions/workflows/ci.yml/badge.svg)](https://github.com/haribabuk113/goperfcheck/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/haribabuk113/goperfcheck.svg)](https://pkg.go.dev/github.com/haribabuk113/goperfcheck)
[![Go Report Card](https://goreportcard.com/badge/github.com/haribabuk113/goperfcheck)](https://goreportcard.com/report/github.com/haribabuk113/goperfcheck)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A zero-dependency static analysis tool that checks Go code against the 18 performance best practices from **[goperf.dev](https://goperf.dev)**.

---

## Install

```bash
go install github.com/haribabuk113/goperfcheck/cmd/goperfcheck@latest
```

Or build from source:
```bash
git clone https://github.com/haribabuk113/goperfcheck.git
cd goperfcheck && make build
```

---

## Quick start

```bash
goperfcheck                         # scan current directory
goperfcheck -severity WARN          # warnings and errors only
goperfcheck -group memory           # run the memory checker group
goperfcheck -git-staged             # check only staged files (pre-commit)
goperfcheck -list-checkers          # show all 18 checkers
goperfcheck -help                   # all flags
```

Output:
```
📁 pkg/handler.go  (2 issue(s))
   [WARN] MemPrealloc:42:10
   ⚠  append() inside a loop — repeated reallocations occur when the backing array runs out of capacity
   💡 Before the loop use make([]T, 0, len(items))
   📊 12 allocs/op → 0; ~10× faster at N=1000

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Found 12 performance issue(s): 1 ERROR · 7 WARN · 4 INFO
```

---

## What it checks

| Checker | Sev | Group | What it catches |
|---------|-----|-------|-----------------|
| MemPrealloc | WARN | memory | `append()` in loops without capacity; `make(map)` without size hint |
| ObjectPool | WARN | memory | High-churn allocations in loops — use `sync.Pool` |
| StructAlign | WARN | memory | Struct fields ordered small→large, wasting padding |
| InterfaceBoxing | INFO | memory | `[]interface{}` params causing heap boxing |
| LazyInit | INFO | memory | Expensive `init()` / package-level allocations |
| StackAlloc | INFO | memory | `new(T)` on primitives forcing heap allocation |
| StringConcatLoop | WARN | memory | `s +=` in a loop — O(n²) copies; use `strings.Builder` |
| RegexpCompile | WARN | memory | `regexp.MustCompile` inside a function — compile once at package level |
| GoroutinePool | WARN | concurrency | Unbounded goroutine creation in loops |
| ContextMisuse | ERROR | concurrency | `context.Context` stored in struct fields |
| AtomicMutex | INFO | concurrency | Simple counters using mutexes — prefer `sync/atomic` |
| TimeNowLoop | INFO | concurrency | `time.Now()` inside a loop — each call is a syscall |
| WaitGroupMisuse | ERROR | concurrency | `wg.Add()` inside a goroutine literal — race condition |
| DeferInLoop | WARN | concurrency | `defer` in a loop fires at function return, not iteration end |
| ZeroCopy | INFO | io | `append([]byte{}, src...)` — unnecessary buffer copy |
| BufferedIO | WARN | io | Unbuffered writes in loops; missing `Flush()` |
| Batching | WARN | io | Individual DB/Redis/HTTP calls in loops |
| HTTPClientReuse | WARN | io | `http.Client{}` per request — abandons connection pool |

→ **[Full checker reference with examples and benchmarks](docs/checkers.md)**

---

## GitHub Actions

Add to any Go repository's CI in two lines — no binary download, no Docker:

```yaml
- uses: actions/setup-go@v5
  with: { go-version: stable }
- uses: haribabuk113/goperfcheck@v1
```

With SARIF upload for inline PR annotations:

```yaml
- uses: haribabuk113/goperfcheck@v1
  with:
    go-version: stable
    sarif-file: results.sarif
  continue-on-error: true

- uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: results.sarif
```

| Input | Default | Description |
|-------|---------|-------------|
| `dir` | `.` | Directory to scan |
| `severity` | `WARN` | Minimum severity: `INFO` / `WARN` / `ERROR` |
| `sarif-file` | `""` | SARIF 2.1.0 output path |
| `args` | `""` | Extra flags, e.g. `-skip-tests -workers 2` |
| `version` | `latest` | goperfcheck version to install |
| `go-version` | `""` | Go version to install; skip if Go is already in PATH |

---

## Documentation

| | |
|---|---|
| [docs/checkers.md](docs/checkers.md) | Per-checker detail, capacity-hint inference, groups, how it works, limitations |
| [docs/usage.md](docs/usage.md) | All flags, output formats, config file, suppression, cache, fix, SARIF |
| [CHANGELOG.md](CHANGELOG.md) | Release history |
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to add a checker, code style, PR process |

---

## Design

- **No external dependencies** — only the Go standard library
- **AST-only** — no `go/types`; fast, no build configuration required
- **Parallel scanning** — one worker per CPU, each with its own `token.FileSet`
- **Content-addressed cache** — `.goperfcheck-cache/` makes repeated runs near-instant
- **Inline suppression** — `//goperfcheck:ignore CheckerName` per line
- **Auto-fix** — `-fix` rewrites `MemPrealloc` findings in place

See **[goperf.dev](https://goperf.dev)** for the underlying performance principles.
