# Changelog

All notable changes to this project will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).  
This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

---

## [0.1.0] — 2026-05-06

### Added
- 12 AST-based performance checkers covering memory preallocation, object
  pooling, struct alignment, interface boxing, zero-copy I/O, goroutine pools,
  context misuse, buffered I/O, atomic vs mutex, lazy initialisation, stack
  allocation, and batching operations
- `-dir` flag to scan a specific directory (default: current directory)
- `-severity` flag to filter output by minimum severity (`INFO` | `WARN` | `ERROR`)
- `-skip-tests` flag to exclude `*_test.go` files from analysis
- `-skip-vendor` flag (default on) to skip the `vendor/` directory
- `-git-staged` flag to check only Go files staged for the next git commit —
  useful as a pre-commit hook
- `-output` flag to write a categorised Markdown report instead of printing to
  stdout; the report includes a summary table and per-category detail sections
- `MemPrealloc` checker now infers capacity hints from the surrounding AST:
  - `range items` → `make([]T, 0, len(items))`
  - `i < n` → `make([]T, 0, n)`
  - `i <= n` → `make([]T, 0, n+1)`
  - `make(map)` followed by a range loop that populates it → `make(map[K]V, len(items))`
  - Falls back to a conservative default of `8` when no size can be inferred

[Unreleased]: https://github.com/haribabuk113/goperfcheck/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/haribabuk113/goperfcheck/releases/tag/v0.1.0
