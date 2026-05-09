# Changelog

All notable changes to this project will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).  
This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
- `-skip-generated` flag (default `true`): silently skips files carrying the
  standard `// Code generated` header during directory and `-git-staged` scans.
  Detected via the already-parsed AST — zero extra file I/O. Set
  `-skip-generated=false` to include generated files. Has no effect on explicit
  `-file` or `-stdin` inputs (user intent takes precedence)
- `-exclude` flag: comma-separated exclusion patterns applied before scanning.
  Patterns ending with `/` skip entire directories during the walk (e.g. `mocks/`,
  `testdata/`); other patterns are matched against the file's base name using
  `filepath.Match` (e.g. `*_gen.go`, `*.pb.go`). Multiple patterns:
  `-exclude 'mocks/,*_gen.go,*.pb.go'`. Applies to directory and `-git-staged`
  scans; explicit `-file` and `-stdin` are always checked
- ANSI color by severity in terminal output: `[ERROR]` is bold red, `[WARN]` is bold
  yellow, `[INFO]` is dim/grey. Colors apply to both the per-issue severity label and
  the footer breakdown (`3 ERROR · 8 WARN · 9 INFO`). Enabled automatically when stdout
  is a terminal; suppressed when piped or redirected so shell pipelines stay clean.
  Disabled entirely by `-no-color` or `NO_COLOR`. Implemented without external
  dependencies using `os.Stdout.Stat()` for tty detection
- `-list-checkers` flag: prints a table of every checker with its name, primary
  severity, theme group, and a one-line description of what it catches — the tool
  is now self-documenting without needing the README. Metadata lives in the exported
  `checker.Metadata` map so library consumers can use it too
- Severity breakdown in the summary footer: the final line now reads
  `Found 20 performance issue(s): 3 ERROR · 8 WARN · 9 INFO` so urgent issues
  are visible without scrolling back through all findings
- Per-file issue count in the text output header: each file line now shows
  `📁 path/to/file.go  (N issue(s))` so you can see the total at a glance without
  scrolling through all findings
- Configuration file (`.goperfcheck`): place a `key = value` file at the repo root
  (or any parent directory) to commit team-wide settings. Parsed before CLI flags so
  explicit flags always take precedence. Supports all persistent settings (`severity`,
  `skip-tests`, `skip-vendor`, `workers`, `no-color`, `group`, `checker`, `format`).
  Inline `#` comments are stripped; unknown keys warn to stderr and are ignored.
  Discovery walks up from the current working directory to the filesystem root, matching
  the convention used by `.gitignore` and `.editorconfig`
- `-group` flag to run a themed subset of checkers in one shot:
  - `memory` — MemPrealloc, ObjectPool, StructAlign, InterfaceBoxing, LazyInit, StackAlloc
  - `concurrency` — GoroutinePool, ContextMisuse, AtomicMutex, TimeNowLoop, WaitGroupMisuse
  - `io` — ZeroCopy, BufferedIO, Batching

  Group resolution is case-insensitive. An unknown group name prints all valid groups
  with their member checkers. `-group` and `-checker` are mutually exclusive.
  `checker.Groups`, `checker.SortedGroupNames()`, and `checker.CheckersForGroup()` are
  exported so library consumers can use the same grouping
- `-stdin` flag: reads a Go source file from stdin instead of walking a directory or
  opening a named file. Issues are reported with `<stdin>` as the filename. Enables
  editor pipe integrations (`:!goperfcheck -stdin` in Vim, `cat foo.go | goperfcheck
  -stdin -format json` for shell pipelines). Compatible with `-format`, `-output`,
  `-checker`, and `-severity`; mutually exclusive with `-file`, `-git-staged`, and
  `-fix`
- `-no-color` flag and `NO_COLOR` environment variable support: disables emoji and
  Unicode box-drawing characters in stdout output, producing plain-text output
  compatible with CI log viewers that mishandle multi-byte Unicode (Jenkins, some
  GitLab runners, etc.). Respects the [NO_COLOR](https://no-color.org) standard —
  setting `NO_COLOR` to any value has the same effect as passing `-no-color`
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

- `DeferInLoop` checker (WARN): flags `defer` statements inside `for`/`range` loops.
  Each iteration allocates a closure on the heap and the defer fires at function
  return (not loop iteration end), which delays resource cleanup and is a common
  correctness bug. Suggests wrapping the loop body in an immediately-invoked
  function literal (`func() { defer f.Close(); ... }()`) or tracking resources
  in a slice and cleaning up after the loop. FuncLit boundaries reset the loop
  context so defer inside a goroutine closure is not flagged
- `StringConcatLoop` checker (WARN): flags `s += expr` and `s = s + expr` inside
  `for`/`range` loops. String concatenation in a loop is O(n²) in allocations —
  each iteration allocates a new string and copies all previous bytes. Suggests
  `strings.Builder` with an upfront `Grow` call, which is O(n) with a single
  allocation. Detects both `+=` tokens and explicit `s = s + x` binary expressions
  where the LHS variable appears on either side of the `+`
- `RegexpCompile` checker (WARN): flags `regexp.Compile`, `regexp.MustCompile`,
  `regexp.CompilePOSIX`, and `regexp.MustCompilePOSIX` calls inside function bodies
  (including function literals). Pattern compilation takes ~microseconds and allocates;
  repeating it on every call is wasteful. Suggests a package-level
  `var re = regexp.MustCompile(...)` so the pattern is compiled once at program start.
  Correctly ignores package-level `var` and `init()` declarations
- `HTTPClientReuse` checker (WARN): flags `http.Client{...}` composite literals
  inside function bodies. Each `http.Client` has its own `Transport` with a fresh
  connection pool; creating a new client per request discards idle TCP/TLS connections
  and forces a new handshake every time (~3 ms vs ~200 µs for a reused connection).
  Suggests a shared package-level `var client = &http.Client{...}`. Only flags the
  composite literal, not variable references, so function parameters/return values are
  not affected
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
- `action.yml` — GitHub Actions marketplace integration. A composite action that
  installs goperfcheck via `go install` and runs it. Inputs: `dir`, `severity`,
  `sarif-file` (writes SARIF 2.1.0 for upload to GitHub Code Scanning),
  `args` (extra flags), `version` (pin a release), `go-version` (optional
  `actions/setup-go` bootstrap). Works on `ubuntu-latest`, `macos-latest`, and
  `windows-latest`. Users can add goperfcheck to any CI pipeline with two lines:
  `uses: haribabuk113/goperfcheck@v1`. Added `.github/workflows/action-test.yml`
  to self-test the composite action on every push.
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
