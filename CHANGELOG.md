# Changelog

All notable changes to this project will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).  
This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Fixed
- **`-file` with a subdirectory path now shows the relative path in output**:
  previously, `-file dir/file.go` displayed the full absolute path (e.g.
  `/home/user/project/dir/file.go`) instead of the expected `dir/file.go`.
  The bug was that terminal output computed `filepath.Rel(".", absolutePath)`,
  which always fails because one operand is relative and the other is absolute.
  The Markdown report had the same defect. SARIF output was already correct.

  Fix: both the terminal output and the Markdown report now call
  `filepath.Abs(*dir)` first and use the resulting absolute base path for
  `filepath.Rel`, matching the approach already used in `writeSARIFReport`.

  ```
  # Before fix
  goperfcheck -file pkg/server/handler.go
  📁 /home/user/project/pkg/server/handler.go  (2 issue(s))

  # After fix
  goperfcheck -file pkg/server/handler.go
  📁 pkg/server/handler.go  (2 issue(s))
  ```

- **`MemPrealloc` no longer re-flags issues that were already fixed by `-fix`**:
  previously, running `goperfcheck -fix` rewrote `var out []string` to
  `out := make([]string, 0, len(items))`, but a subsequent `goperfcheck` run
  reported the same issue again. The checker flagged `append()` inside a loop
  unconditionally — it did not check whether the accumulation variable was
  already pre-allocated.

  The checker now precomputes all variables declared with `make([]T, len, cap)`
  (3-argument make with a slice type) anywhere in the file. When processing an
  `append(x, ...)` call inside a loop, it checks whether `x` was declared with
  a capacity hint at any source position before the loop starts. If so, the issue
  is suppressed — the code is already correct and no fix is needed.

  Behaviour summary:
  ```go
  // Before fix — flagged correctly:
  var out []string
  for _, v := range items { out = append(out, v) }

  // After -fix — NOT flagged (already preallocated):
  out := make([]string, 0, len(items))
  for _, v := range items { out = append(out, v) }

  // make([]T, 0) with zero cap — still flagged (cap=0 is same as var decl):
  out := make([]string, 0)
  for _, v := range items { out = append(out, v) }
  ```

  Cross-scope declarations (variable declared in an outer block before the
  function's loop) are also handled correctly.

### Added
- **Clickable file links in terminal output**: every issue line now shows
  `rel/path/to/file.go:line:col` as the file location, placed between the
  severity tag and the checker name. In terminals that support OSC 8
  hyperlinks (iTerm2, WezTerm, GNOME Terminal 3.26+, VS Code integrated
  terminal, Kitty), the location is a clickable link that opens the file
  directly. Plain-text output (piped, CI, `-output` files) is unaffected —
  the escape codes are omitted when stdout is not a TTY or color is disabled.

  Before:
  ```
     [WARN] MemPrealloc:10:5
  ```
  After:
  ```
     [WARN] example.go:10:5  MemPrealloc
  ```

### Fixed
- **`-cache false` now works as expected**: previously, `-cache false`
  (space-separated) silently left the cache enabled and treated `false` as a
  positional argument — a footgun caused by how Go's `flag` package handles
  boolean flags (`IsBoolFlag() == true` means the flag may appear without a
  value, so the parser does not consume the next token as its value).

  A pre-parse normalization step (`normalizeSpacedBoolFlags`) now rewrites
  `-flag value` → `-flag=value` for every registered boolean flag whenever
  `value` is a bare boolean literal (`true`, `false`, `1`, `0`). Both forms
  are now equivalent:

  ```
  goperfcheck -cache false     # ✓ now works (space-separated)
  goperfcheck -cache=false     # ✓ always worked (equals form)
  ```

  The fix is generic and covers all boolean flags (`-skip-vendor`,
  `-skip-tests`, `-skip-generated`, `-git-staged`, `-git-diff`, etc.) —
  any bool flag with a default of `true` that a user might want to negate.
  Flag defaults and all other behaviour are unchanged.

### Added
- **`-fix` now rewrites `StructAlign` issues (field reordering)**: the auto-fix
  flag previously only handled `MemPrealloc` (slice/map capacity hints). It now
  also rewrites struct field ordering for any struct flagged by `StructAlign`.

  The rewrite sorts fields **largest → smallest** (by `approxSize`), which
  eliminates padding waste without changing the struct's total field set or
  removing any comments, tags, or doc-comment blocks. Specifically:

  - **Struct tags are preserved** — each field's tag stays on the same line as
    the field, moved together.
  - **Inline comments are preserved** — trailing `// comment` lines move with
    their field.
  - **Doc comment blocks are preserved** — multi-line `// doc` blocks that
    immediately precede a field are treated as part of that field and reordered
    with it.
  - **`gofmt`-clean output** — the reordered source is formatted via
    `go/format.Source` before write.
  - **Exported structs** — the fix is applied for both exported and unexported
    structs. Exported structs already carry the API-safety caveat in the issue
    suggestion ("audit callers for positional struct literals before
    reordering"). The user opts in to the risk by passing `-fix`.

  Implementation notes:
  - `FixHint` gains two new fields: `StructName string` (type name) and
    `FieldOrder []int` (pre-computed sorted field indices). The checker
    computes the optimal order once at detection time; the fixer only needs
    to reorder lines.
  - The fixer works at the source-line level using `token.File.LineStart` to
    map AST positions to byte offsets, so it handles any indentation style.
  - New `fix.go` function: `structReorderEdit`.

- **`-git-diff` flag — diff-aware scanning**: `goperfcheck -git-diff` checks only
  the lines that were added or modified compared to HEAD. It combines staged and
  unstaged working-tree changes (`git diff HEAD`) so a single flag covers the full
  set of edits since the last commit, without re-checking unchanged code.

  How it differs from `-git-staged`:

  | Flag | What is checked |
  |------|----------------|
  | `-git-staged` | All lines of every **staged file** |
  | `-git-diff` | Only the **added/modified lines** across both staged and unstaged files |

  Use `-git-diff` in editors and pre-save hooks where you want instant, noise-free
  feedback scoped to exactly what you just wrote. Use `-git-staged` in pre-commit
  hooks where you want full coverage of every file you are about to commit.

  Implementation notes:
  - Parses the unified diff from `git diff HEAD` to build a per-file set of
    changed line numbers; issues are filtered to that set after scanning.
  - Newly created files (`git add`-ed) appear as fully-added in the diff, so all
    issues in them are reported.
  - Untracked files (not yet `git add`-ed) are not covered; `git add` them first.
  - Conflicts with `-git-staged`, `-stdin`, and `-file` (exits with an error).
  - When the diff contains no `.go` files, exits cleanly with
    `No Go changes detected in working tree or index`.

### Fixed
- **Unknown subcommand rejected**: positional arguments that are not a recognised
  subcommand (e.g. `goperfcheck help`, `goperfcheck version`) previously fell
  through silently and triggered a full directory scan of `.`. They now print a
  clear error message (`error: unknown subcommand "help"`) and exit with code 2.
  The error output also lists the only known subcommand (`explain`) and points to
  `goperfcheck -help` for flag usage. The `explain` subcommand and all `-flags`
  are unaffected.

- **Markdown report benchmark on new line**: in the `-output report.md` format,
  the `📊 <benchmark>` line was rendering on the same line as `💡 <suggestion>`
  because the suggestion line lacked the two trailing spaces required for a
  Markdown hard line break. Added `  \n` (two spaces + newline) after the
  suggestion so the benchmark datum always appears on its own line in rendered
  Markdown.

### Added
- **Go version awareness**: goperfcheck now reads the `go X.Y` directive from
  `go.mod` (walking up from the scan root, same discovery as `.goperfcheck`
  config) and adjusts advice for version-sensitive checkers. Use
  `-go-version X.Y` to override — useful in pipelines where no `go.mod` is
  present. When no version is detected and the flag is not set, a tip is printed
  suggesting the flag. Detected version is printed at the end of every scan.

  Four checkers are version-sensitive:

  | Checker | Threshold | Adjustment |
  |---------|-----------|------------|
  | **GoroutinePool** | ≥ 1.22 | Go 1.22 per-iteration loop variables eliminate the classic closure capture correctness bug; adds a `📦` note clarifying the finding is now a performance concern only |
  | **StackAlloc** | ≥ 1.17 | Escape analysis improvements mean the compiler may already stack-allocate the flagged patterns; adds a `📦` note with a verification command (`go build -gcflags='-m' ./...`) |
  | **ZeroCopy** | < 1.20 | `bytes.Clone()` does not exist before Go 1.20; adds a `📦` note replacing the suggestion with `make([]byte, len(src)); copy(...)` |
  | **AtomicMutex** | < 1.19 | `atomic.Int64`, `atomic.Bool`, `atomic.Uint64` (typed atomics) do not exist before Go 1.19; **rewrites the suggestion** from the typed-atomic form to the function-based API (`atomic.AddInt64` / `StoreInt64` / `LoadInt64`) — the previous suggestion was actively wrong on older projects |

  New exported API: `checker.GoVersion`, `checker.ParseGoVersion`,
  `checker.ReadModGoVersion`, `checker.ApplyGoVersion`. The `Issue` struct
  gains a new `VersionNote string` field (JSON: `"version_note"`, `omitempty`)
  populated by `ApplyGoVersion` when advice changes for the detected version.

- **Checker confidence levels**: every checker now carries a `Confidence` field
  (`HIGH` / `MEDIUM` / `LOW`) and a `FalsePositiveNote` explaining the specific
  AST limitation that causes false positives. Because goperfcheck is AST-only
  with no type resolution, some findings fire on patterns that look wrong
  syntactically but are correct in context — confidence makes this explicit
  rather than hiding it in fine print.

  | Confidence | Meaning |
  |-----------|---------|
  | HIGH | Structurally reliable; safe to act immediately |
  | MEDIUM | Usually correct; review surrounding context first |
  | LOW | Fires on legitimate code patterns; verify before acting |

  - **AtomicMutex** and **InterfaceBoxing** are rated LOW. AtomicMutex cannot
    determine whether a mutex also guards non-atomic state outside the struct —
    following the suggestion when it does would introduce data races.
    InterfaceBoxing cannot distinguish unavoidable boxing (`fmt.Fprintf`,
    `json.Marshal`, variadic `...any`) from optimisable cases.
  - Nine checkers are MEDIUM (pattern usually correct, edge cases documented).
  - Eight checkers remain HIGH (structural guarantees are sufficient).

  **How confidence is surfaced:**
  - **`-list-checkers`** gains a `CONF` column (color-coded green/yellow/red on
    terminals; plain text with `-no-color`).
  - **`explain <CheckerName>`** shows a "Confidence" line and, for MEDIUM/LOW
    checkers, a "Limitations (AST-only analysis)" section with the full note.
  - **Terminal output** appends a `🔍` line (`fp?:` in plain mode) for every LOW
    confidence finding, naming the specific limitation so the developer knows
    exactly what to verify before making a change.
  - `CheckerMeta.Confidence` and `CheckerMeta.FalsePositiveNote` are exported so
    library consumers and editor integrations can surface the same information.

- **False-positive test coverage**: three new/updated test functions pin the
  behaviour of known false-positive scenarios so regressions are caught and the
  limitations are visible in the test suite:
  - `TestAtomicMutexFalsePositive_MutexGuardsMultipleFields` — fires on a struct
    whose mutex could guard external state; documents the data-race risk
  - `TestInterfaceBoxingFalsePositive_UnavoidableBoxing` — three sub-cases: a
    fmt-style variadic wrapper, a json-marshaling helper, and an intentional
    heterogeneous container; all fire, all are documented as correct false
    positives
  - `TestMemPreallocFalsePositive_LoopLocalSlice` — fires on a loop-local slice
    inside a range loop alongside the outer accumulation slice; documents that
    the checker cannot distinguish which slice is the actual problem

- **Suppression expiry (`until:`)**: `//goperfcheck:ignore MemPrealloc until:2026-09-01`
  attaches a date-bound lifetime to any suppression comment. When the date passes
  the suppression automatically stops hiding the issue — the checker finding resurfaces
  in normal output so the team is prompted to re-evaluate the decision. Works with
  both single-checker and wildcard (`//goperfcheck:ignore until:DATE`) suppression
  forms. Date is parsed as `YYYY-MM-DD`; invalid dates are silently ignored
  (suppression remains active). Implemented inside `FilterSuppressed` with no
  signature change — zero impact on existing code or cached results.

- **`-audit-suppressions` flag**: scans all Go files, runs every checker without
  suppression filtering, and cross-references the raw findings against
  `//goperfcheck:ignore` comments to produce a suppression health report.
  A suppression is **stale** when none of its covered checkers fire on the annotated
  line(s), and **expired** when its `until:` date has passed. Output is grouped by
  file, listing only problematic entries with their reconstructed comment text.
  Exit code 0 = all suppressions healthy; exit code 1 = stale or expired entries
  found — suitable as a periodic CI job. Always runs all checkers regardless of
  `-checker`/`-group` filters so stale detection is complete and accurate.
  Incompatible with `-stdin` (suppressions in ephemeral input have no persistence
  value).

  Example output:
  ```
  Suppression audit
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Found 2 problematic suppression(s) of 7 total (2 stale, 1 expired).

  📁 pkg/handler.go
     line 42   [STALE]    //goperfcheck:ignore MemPrealloc
     line 87   [STALE]    //goperfcheck:ignore

  📁 pkg/cache.go
     line 103  [EXPIRED]  //goperfcheck:ignore StructAlign until:2026-04-01

  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Stale: the suppressed checker no longer fires — safe to remove the comment.
  Expired: the until: date has passed — revisit and remove or renew the suppression.
  ```

### Changed
- **StructAlign**: the suggestion now branches on whether the flagged struct is
  exported. For unexported types the suggestion is unchanged. For exported types
  it reads: *"Exported type — audit callers for positional struct literals
  (T{v1, v2, …}) before reordering; reordering exported fields is a breaking
  API change for any caller that omits field names. If all call sites use named
  fields: reorder largest → smallest, int64/pointers first, then int32, int16,
  bool/byte last"* — preventing a silent API break when a team follows the
  suggestion on a public library. Detection is purely name-based (uppercase first
  character); the check does not depend on build tags or module visibility.
  `docs/checkers.md` gains a dedicated `StructAlign` section explaining the
  exported-type risk, how to grep for positional literals, and the suppression
  comment pattern.

### Added
- **SyncMapMisuse** checker (WARN, concurrency group): flags `sync.Map` struct
  fields, `var m sync.Map` declarations, and `m := sync.Map{}` short declarations.
  `sync.Map` is only faster than `map[K]V + sync.RWMutex` in two narrow cases —
  append-only caches (written once, read many times) and per-goroutine disjoint
  key sets. In all other patterns it is measurably slower and boxes every
  key/value as `interface{}`, paying 3 heap allocations per `Store` vs 0 for
  `map+RWMutex`. Benchmark: sequential store+load 662 ns/op, 3 allocs/op vs
  308 ns/op, 0 allocs/op. Includes a `benchmarks/sync_map_misuse_bench_test.go`
  with sequential (misuse) and high-contention read-only (legitimate) pairs.
- `explain` subcommand: `goperfcheck explain <CheckerName>` prints what a checker
  looks for, a short bad/good code example, and the goperf.dev reference URL —
  making the tool self-documenting at the terminal without needing the README.
  Checker names are case-insensitive. Running `goperfcheck explain` with no
  argument lists all available checker names with their one-line descriptions.
  Respects `-no-color` and `NO_COLOR` for the severity label color.
- `CheckerMeta` now carries three new exported fields: `Link` (goperf.dev URL),
  `BadExample`, and `GoodExample` (code snippets used by `explain`). All 19
  checkers are populated.

---

## [0.2.0] — 2026-05-09

### Added
- `docs/` directory: `README.md` trimmed from 528 → 137 lines and now serves
  as a quick-start page. Full content moved to `docs/checkers.md` (per-checker
  detail, capacity hint inference, groups, limitations) and `docs/usage.md`
  (all flags, output formats, config file, suppression, cache, SARIF).
  `CONTRIBUTING.md` updated to point contributors at the right files
- Progress indicator on stderr: when scanning ≥ 50 files and stderr is an
  interactive terminal, a live `Scanning... (N/Total files)` line is printed and
  updated every 150 ms using carriage-return overwriting. The line is erased
  before results are printed, so the final output is always clean. Suppressed
  automatically when stderr is piped or redirected (CI logs stay clean), and not
  shown for `-stdin` or tiny repos where scanning completes in under 150 ms
- Result cache (`.goperfcheck-cache/`): parse and check results are stored in a
  content-addressed on-disk cache keyed by `SHA-256(file content)` +
  `SHA-256(tool version + active checker names)`. On a cache hit the file is
  read but never re-parsed or re-checked — repeated runs on unchanged files are
  near-instant regardless of repo size. The config hash covers the tool version
  and the exact set of checkers being run, so changing `-checker`, `-group`, or
  upgrading the binary automatically invalidates stale entries. Cache writes are
  atomic (temp-file rename) and safe under concurrent workers. Add
  `.goperfcheck-cache/` to `.gitignore` — the directory should not be committed.
  Disable with `-cache=false` or set `cache = false` in `.goperfcheck`
- `-cache` flag (default `true`): enable or disable the result cache. Useful for
  one-shot CI runs where caching is not needed and the directory should stay clean
- Fixed a pre-existing bug in the default directory walk: `filepath.Base(".")` is
  `"."`, causing `strings.HasPrefix(".", ".")` to return true and immediately skip
  the entire tree with `filepath.SkipDir` when `-dir .` (the default) was used.
  The root path is now excluded from the hidden-directory filter, so
  `goperfcheck` (no `-dir` flag) correctly scans the current directory tree
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

[0.2.0]: https://github.com/haribabuk113/goperfcheck/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/haribabuk113/goperfcheck/releases/tag/v0.1.0
