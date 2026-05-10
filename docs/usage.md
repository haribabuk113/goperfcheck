# Usage Guide

## Common invocations

```bash
goperfcheck                          # scan current directory, all checkers, severity ≥ INFO
goperfcheck -dir ./pkg               # scan a specific directory
goperfcheck -severity WARN           # show only WARN and ERROR
goperfcheck -severity ERROR          # show only ERROR (critical issues)
goperfcheck -file ./pkg/handler.go   # check a single file
goperfcheck -git-staged              # check only files staged for commit (pre-commit hook)
goperfcheck -list-checkers           # print all checkers with group and description
goperfcheck explain MemPrealloc      # show what a checker looks for + bad/good examples
goperfcheck -version                 # print version
goperfcheck -help                    # print all flags
```

---

## Flags reference

| Flag | Default | Description |
|------|---------|-------------|
| `-dir` | `.` | Root directory to scan |
| `-file` | `""` | Check a single `.go` file |
| `-checker` | `""` | Run only the named checker (case-insensitive) |
| `-group` | `""` | Run only checkers in a theme: `memory` / `concurrency` / `io` |
| `-severity` | `INFO` | Minimum severity to report: `INFO` / `WARN` / `ERROR` |
| `-skip-tests` | `false` | Skip `*_test.go` files |
| `-skip-vendor` | `true` | Skip the `vendor/` directory |
| `-skip-generated` | `true` | Skip files with a `// Code generated` header |
| `-exclude` | `""` | Comma-separated exclusion patterns (see below) |
| `-git-staged` | `false` | Check only files staged for the next git commit |
| `-output` | `""` | Write report to file: `.md` → Markdown, `.sarif` → SARIF 2.1.0 |
| `-format` | `text` | stdout format: `text` or `json` |
| `-workers` | `NumCPU` | Number of parallel goroutines for file scanning |
| `-fix` | `false` | Auto-apply fixable suggestions in place |
| `-cache` | `true` | Cache parse+check results in `.goperfcheck-cache/` |
| `-stdin` | `false` | Read Go source from stdin |
| `-no-color` | `false` | Disable ANSI color and emoji (also: `NO_COLOR` env var) |
| `-list-checkers` | `false` | Print checker table and exit |
| `-audit-suppressions` | `false` | Report stale and expired `//goperfcheck:ignore` comments, then exit |
| `-version` | `false` | Print version and exit |

---

## Filtering and exclusion

### By severity
```bash
goperfcheck -severity WARN    # WARN + ERROR only
goperfcheck -severity ERROR   # ERROR only
```

### Skip test and generated files
```bash
goperfcheck -skip-tests                  # exclude *_test.go
goperfcheck -skip-generated=false        # include files with '// Code generated' header
```

### Exclude directories and globs
```bash
goperfcheck -exclude 'mocks/'                    # skip the mocks/ directory
goperfcheck -exclude '*_gen.go'                  # skip files matching glob
goperfcheck -exclude 'mocks/,*_gen.go,*.pb.go'  # combine with commas
```

Pattern rules:
- Patterns ending with `/` match directory names — the whole subtree is skipped during the walk
- All other patterns are matched against the file's base name using `filepath.Match`

Exclusion applies to directory and `-git-staged` scans. Explicit `-file` and `-stdin` inputs are always checked.

---

## Output formats

### Terminal text (default)
```
📁 pkg/handler.go  (2 issue(s))
   [WARN] MemPrealloc:42:10
   ⚠  append() inside a loop — repeated reallocations occur when the backing array runs out of capacity
   💡 Before the loop use make([]T, 0, len(items))
   📊 12 allocs/op → 0; ~10× faster at N=1000

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Found 2 performance issue(s): 0 ERROR · 1 WARN · 1 INFO
```

Severity labels are color-coded when stdout is a terminal (`[ERROR]` bold red,
`[WARN]` bold yellow, `[INFO]` dim grey). Colors are suppressed automatically
when stdout is piped or redirected.

### Plain text (`-no-color` / `NO_COLOR`)
```bash
goperfcheck -no-color
NO_COLOR=1 goperfcheck
```
Disables emoji, Unicode box-drawing, and ANSI colors. Useful in CI log viewers
that mishandle terminal control codes. Respects the [NO_COLOR](https://no-color.org) standard.

### JSON (`-format json`)
```bash
goperfcheck -format json
goperfcheck -file ./pkg/cache.go -format json | jq '.[].message'
```
Emits a JSON array of issue objects to stdout. Fields: `checker`, `file`, `line`,
`column`, `severity`, `message`, `rule`, `suggestion`, `benchmark`. Exit codes
unchanged (0 = clean, 1 = issues found, 2 = tool error).

### Markdown report (`-output report.md`)
```bash
goperfcheck -output report.md
goperfcheck -dir ./pkg -output report.md -severity WARN
```
Writes a categorised report with a summary table and per-category detail sections.
Exit code 1 when issues are found; the report is written regardless.

### SARIF 2.1.0 (`-output results.sarif`)
```bash
goperfcheck -output results.sarif
```
When the output path ends in `.sarif`, the report is written as SARIF 2.1.0
instead of Markdown. Upload to GitHub for inline PR annotations:

```yaml
- name: Run goperfcheck
  run: goperfcheck -dir . -output results.sarif
  continue-on-error: true

- uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: results.sarif
```

---

## Checker confidence levels

Because goperfcheck is AST-only with no type resolution, some checkers fire on
patterns that look wrong in the syntax tree but are correct in context. Each
checker carries a **confidence level** that tells you how often to expect a
false positive:

| Confidence | Meaning | When to act |
|-----------|---------|-------------|
| **HIGH** | Structurally reliable — the syntax alone is sufficient to conclude a problem | Safe to act immediately |
| **MEDIUM** | Usually correct, known edge cases | Review the surrounding code first |
| **LOW** | Fires on patterns that frequently appear in legitimate code | Verify manually before making any change |

Confidence is visible in:
- `-list-checkers` — CONF column (color-coded: green/yellow/red when output is a terminal)
- `explain <CheckerName>` — "Confidence" line plus a "Limitations" section for MEDIUM and LOW checkers
- **Terminal output** — LOW confidence findings include a `🔍` line describing the specific AST limitation so you know what to verify

### Which checkers are LOW confidence and why

**AtomicMutex** (LOW): Cannot determine whether the mutex guards non-atomic state
outside the struct definition. The struct might have a companion mutex protecting
a slice — converting to atomics in that case introduces a data race.

**InterfaceBoxing** (LOW): Cannot distinguish unavoidable boxing (`fmt.Fprintf`,
`json.Marshal`, variadic `...any` wrappers) from optimisable cases. Also fires on
intentionally heterogeneous containers where `[]any` is the correct choice.

### Example: LOW confidence finding in terminal output

```
📁 pkg/counter.go  (1 issue(s))
   [INFO] AtomicMutex:12:2
   ⚠  struct "Counter" uses sync.Mutex to protect only atomic-compatible scalar fields
   💡 Replace sync.Mutex with atomic.Int64, atomic.Bool, or atomic.Uint64
   📊 ~4× faster under 8-goroutine contention
   🔍 Low confidence: Cannot see if this mutex also guards non-atomic state (slices,
      maps, pointers) outside this struct. If it does, converting would introduce
      data races. Run 'explain AtomicMutex' for details.
```

---

## explain subcommand

`explain` is the fastest way to understand a finding without leaving the terminal.
It shows what the checker looks for, the goperf.dev reference, and a short bad/good
code example — making it easy to decide whether a result is a false positive.

```bash
goperfcheck explain MemPrealloc      # by exact name
goperfcheck explain memprealloc      # case-insensitive
goperfcheck explain                  # list all checker names
```

Example output:

```
Checker:     MemPrealloc
Severity:    [WARN]
Group:       memory
Description: append() in loops without capacity; make(map) without size hint
Docs:        https://goperf.dev/01-common-patterns/mem-prealloc/

─────────────────────────────────────────────────
Bad (will trigger):
  var result []string
  for _, v := range items {
      result = append(result, v) // reallocates repeatedly
  }

─────────────────────────────────────────────────
Good (preferred):
  result := make([]string, 0, len(items))
  for _, v := range items {
      result = append(result, v)
  }
```

Checker names are case-insensitive. The color of the severity label respects
`-no-color` and `NO_COLOR`.

---

## Inline suppression comments

Place a `//goperfcheck:ignore` comment on the flagged line (or on the `for`/`range`
line to suppress loop-body issues):

```go
// Suppress all checkers on this line:
result = append(result, v) //goperfcheck:ignore

// Suppress a specific checker:
result = append(result, v) //goperfcheck:ignore MemPrealloc

// Suppress multiple checkers:
result = append(result, v) //goperfcheck:ignore MemPrealloc,StructAlign

// For loop-body issues, annotate the for/range statement:
for _, v := range items { //goperfcheck:ignore MemPrealloc
    result = append(result, v)
}
```

### Expiry annotations (`until:`)

Add `until:YYYY-MM-DD` to set an expiry date on a suppression. Once that date
passes the comment stops suppressing — the issue resurfaces automatically so
the team is reminded to re-evaluate the decision:

```go
// Suppress until a planned refactor is merged (then this comment auto-expires):
for _, v := range items { //goperfcheck:ignore MemPrealloc until:2026-09-01
    result = append(result, v)
}

// Works with specific or wildcard suppression:
result = append(result, v) //goperfcheck:ignore until:2026-09-01
```

### Auditing suppressions (`-audit-suppressions`)

Over time suppressions can go stale — the underlying issue gets fixed but the
`//goperfcheck:ignore` comment remains. Run the audit to find dead comments:

```bash
goperfcheck -audit-suppressions          # scan current directory
goperfcheck -audit-suppressions -dir ./pkg
goperfcheck -audit-suppressions -file ./pkg/handler.go
```

Example output when problems are found:

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

| Status | Meaning | Action |
|--------|---------|--------|
| **STALE** | The named checker no longer fires on that line | Remove the comment |
| **EXPIRED** | The `until:` date has passed | Revisit — remove or set a new date |

Exit code 0 when all suppressions are active; exit code 1 when stale or expired
entries are found. Suitable as a periodic CI check:

```yaml
- name: Audit suppression debt
  run: goperfcheck -audit-suppressions -dir .
```

`-audit-suppressions` always runs **all** checkers regardless of `-checker` or
`-group` filters, ensuring stale detection is accurate. It is incompatible with
`-stdin`.

---

## Auto-fix (`-fix`)

```bash
goperfcheck -fix
goperfcheck -file ./pkg/cache.go -fix
```

Rewrites source files in place:
- `make(map[K]V)` → `make(map[K]V, hint)` — inserts the inferred capacity
- `var x []T` immediately before a loop → `x := make([]T, 0, hint)`

Currently supported for `MemPrealloc` findings only. Other issue types are left
unchanged. Edits are applied end-to-start within each file so byte offsets stay
valid; `go/format` normalises whitespace afterward.

> **Caution**: `-fix` modifies source files directly. Commit or back up your work first.

Cannot be combined with `-stdin`.

---

## stdin mode (`-stdin`)

```bash
cat ./pkg/handler.go | goperfcheck -stdin
goperfcheck -stdin < ./pkg/handler.go
goperfcheck -stdin -format json < ./pkg/handler.go | jq '.[].message'
```

Reads a Go source file from stdin. Issues are reported with `<stdin>` as the
filename. Compatible with `-format`, `-output`, `-checker`, and `-severity`.
Cannot be combined with `-file`, `-git-staged`, or `-fix`.

Editor integrations:
- **Vim**: `:!goperfcheck -stdin`
- **shell**: `cat foo.go | goperfcheck -stdin -format json`

---

## Result cache

Parse and check results are cached in `.goperfcheck-cache/` at the scan root,
keyed by `SHA-256(file content)` + a hash of the tool version and active checker
set. Unchanged files are never re-parsed on subsequent runs.

```bash
goperfcheck               # first run: parses all files, populates cache
goperfcheck               # second run: cache hits only — near-instant
goperfcheck -cache=false  # bypass cache (useful in clean CI environments)
```

Add to `.gitignore`:
```
.goperfcheck-cache/
```

Changing `-checker`, `-group`, or upgrading the binary automatically invalidates
stale entries — no manual cache clearing needed. Cache writes use atomic
temp-file rename, safe under concurrent workers.

---

## Parallelism

```bash
goperfcheck                  # default: runtime.NumCPU() workers
goperfcheck -workers 4       # cap at 4 (resource-constrained CI)
goperfcheck -workers 1       # single-threaded, deterministic order
```

Each worker gets its own `token.FileSet` — no lock contention. Output is sorted
by file and line regardless of completion order.

When scanning ≥ 50 files on an interactive terminal, a live progress counter is
printed on stderr (`Scanning... (1240/5000 files)`) and erased before results
appear. Suppressed automatically when stderr is piped or redirected.

---

## Pre-commit hook

```bash
goperfcheck -git-staged -severity WARN
```

Checks only the Go files staged for the next commit. Add to `.git/hooks/pre-commit`:

```bash
#!/bin/sh
goperfcheck -git-staged -severity WARN
```

---

## Configuration file

Create `.goperfcheck` at the repo root (or any parent directory) to commit
team-wide defaults. CLI flags always override config file values.

```ini
# goperfcheck project configuration

severity   = WARN
skip-tests = true
# workers  = 4
# group    = memory
# cache    = false
```

**Discovery**: goperfcheck searches upward from the current working directory to
the filesystem root, so running from any subdirectory finds the same file.

**Supported keys**: `severity`, `skip-tests`, `skip-vendor`, `skip-generated`,
`workers`, `no-color`, `group`, `checker`, `format`, `cache`, `exclude`.

**Not recommended in config**: `fix`, `stdin`, `git-staged`, `file`, `output`
(these are inherently ad-hoc).

Unknown keys produce a warning to stderr and are ignored.
