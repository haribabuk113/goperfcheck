# Changelog

All notable changes to this project will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).  
This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
- `-file` flag to check a single `.go` file instead of walking a directory
- `-checker` flag to run only one named checker (case-insensitive); prints valid
  names if the given name is not recognised
- `-format json` flag to emit all issues as a JSON array on stdout — useful for
  editor integrations and downstream tooling; exit codes unchanged (0 = clean,
  1 = issues, 2 = tool error)
- `-version` flag to print the tool version and exit
- SARIF 2.1.0 output: when `-output` ends in `.sarif` the report is written in
  SARIF format instead of Markdown, enabling native GitHub Code Scanning
  annotations in pull requests via `github/codeql-action/upload-sarif`
- `SECURITY.md` documenting the supported security model and how to report
  vulnerabilities
- `CODE_OF_CONDUCT.md` (Contributor Covenant v2.1)
- `examples/` directory with two ready-to-compile programmatic usage examples:
  `examples/basic/` (full directory scan) and `examples/single_checker/`

- `TimeNowLoop` checker (INFO): flags `time.Now()` calls inside `for`/`range` loops —
  each call is a syscall; suggests caching the value before the loop
- `WaitGroupMisuse` checker (ERROR): flags `wg.Add(n)` called inside a goroutine
  literal (`go func() { wg.Add(1) }()`), which is a race condition — the counter
  must be incremented before the `go` statement. Patterns #1 (string concat), #2
  (regexp compile), and #5 (fmt.Sprintf concat) from the roadmap were deliberately
  not implemented: they are already covered by `perfsprint` and `gocritic` in
  golangci-lint
- Parallel file scanning: the directory walk now fans out across a worker pool
  (default: `runtime.NumCPU()` goroutines). Each worker uses its own
  `token.FileSet` so there is no shared mutable state. Override with
  `-workers N` when you need to cap CPU usage in CI
- Inline suppression comments: `//goperfcheck:ignore` on a line suppresses all
  checkers; `//goperfcheck:ignore CheckerName` suppresses a specific checker;
  `//goperfcheck:ignore A,B` suppresses multiple checkers. The comment can appear
  on the same line as the offending code or on the `for`/`range` statement line
  when suppressing loop-body issues
- `-fix` flag to auto-apply fixable suggestions in place: rewrites `make(map[K]V)`
  → `make(map[K]V, hint)` (always safe, purely additive) and rewrites
  `var x []T` immediately before a loop → `x := make([]T, 0, hint)` (only when the
  declaration has no initializer or an explicit `nil`). Edits are applied
  end-to-start within each file so byte offsets remain valid, followed by
  `go/format` to normalise whitespace. Currently only `MemPrealloc` issues produce
  fix hints; other checkers leave `Fix` nil and are skipped silently.

### Security
- Fixed path traversal vulnerability in `-git-staged` mode: file paths returned
  by `git diff --name-only` that contain `..` after `filepath.Clean` are now
  silently skipped before being joined with the repository root

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
