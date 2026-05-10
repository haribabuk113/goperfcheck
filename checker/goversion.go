package checker

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// GoVersion represents the minimum Go version declared in a module's go.mod.
// The zero value (Major==0, Minor==0) means the version is unknown.
type GoVersion struct {
	Major int
	Minor int
}

// Zero reports whether v is the zero value (version not known).
func (v GoVersion) Zero() bool { return v.Major == 0 && v.Minor == 0 }

// AtLeast reports whether v is at least major.minor.
func (v GoVersion) AtLeast(major, minor int) bool {
	if v.Major != major {
		return v.Major > major
	}
	return v.Minor >= minor
}

// String returns the version in "major.minor" form, e.g. "1.22".
// Returns "unknown" for the zero value.
func (v GoVersion) String() string {
	if v.Zero() {
		return "unknown"
	}
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

// ParseGoVersion parses a version string of the form "1.22", "1.22.0", or
// "1.21rc3". Only the major and minor components are retained.
// An optional leading "go" prefix is accepted (e.g. "go1.22").
func ParseGoVersion(s string) (GoVersion, bool) {
	s = strings.TrimPrefix(strings.ToLower(s), "go")
	parts := strings.SplitN(s, ".", 3)
	if len(parts) < 1 || parts[0] == "" {
		return GoVersion{}, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return GoVersion{}, false
	}
	minor := 0
	if len(parts) >= 2 && parts[1] != "" {
		// Strip any pre-release suffix from the minor component ("21rc3" → "21").
		minorStr := parts[1]
		end := len(minorStr)
		for i, c := range minorStr {
			if c < '0' || c > '9' {
				end = i
				break
			}
		}
		if m, atoiErr := strconv.Atoi(minorStr[:end]); atoiErr == nil {
			minor = m
		}
	}
	return GoVersion{Major: major, Minor: minor}, true
}

// ReadModGoVersion searches for a go.mod file starting from root and walking
// up to the filesystem root. It returns the version from the go directive and
// true on success, or the zero GoVersion and false if no go.mod is found or
// the file has no go directive.
func ReadModGoVersion(root string) (GoVersion, bool) {
	dir, err := filepath.Abs(root)
	if err != nil {
		return GoVersion{}, false
	}
	for {
		if v, ok := parseGoModFile(filepath.Join(dir, "go.mod")); ok {
			return v, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return GoVersion{}, false
}

func parseGoModFile(path string) (GoVersion, bool) {
	f, err := os.Open(path)
	if err != nil {
		return GoVersion{}, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "go ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if v, ok := ParseGoVersion(fields[1]); ok {
			return v, true
		}
	}
	return GoVersion{}, false
}

// ApplyGoVersion adjusts issues whose advice changes or no longer applies for
// the given Go version. When v is the zero value no adjustments are made.
//
// Adjustments:
//   - GoroutinePool ≥ 1.22: loop variables are per-iteration; the classic
//     closure capture correctness bug is gone — finding is performance-only.
//   - StackAlloc ≥ 1.17: escape analysis improvements may already handle
//     many of these patterns — encourage profiling before refactoring.
//   - ZeroCopy < 1.20: bytes.Clone() does not exist; replace suggestion.
//   - AtomicMutex < 1.19: atomic.Int64/Bool/Uint64 (typed atomics) do not
//     exist; fix the struct-level suggestion to use function-based atomics.
func ApplyGoVersion(issues []Issue, v GoVersion) []Issue {
	if v.Zero() {
		return issues
	}
	for i := range issues {
		switch issues[i].Checker {

		case "GoroutinePool":
			if v.AtLeast(1, 22) {
				issues[i].VersionNote = "Go 1.22+: loop variables are per-iteration — " +
					"the closure capture correctness bug is gone. " +
					"This finding is now a performance concern only (unbounded goroutines)."
			}

		case "StackAlloc":
			if v.AtLeast(1, 17) {
				issues[i].VersionNote = fmt.Sprintf(
					"Go %s: escape analysis improvements mean the compiler may already "+
						"stack-allocate this. Verify with `go build -gcflags='-m' ./...` "+
						"before refactoring.", v.String())
			}

		case "ZeroCopy":
			if !v.AtLeast(1, 20) {
				issues[i].VersionNote = fmt.Sprintf(
					"Go %s: bytes.Clone() requires Go 1.20+. "+
						"Use: dst := make([]byte, len(src)); copy(dst, src) instead.",
					v.String())
			}

		case "AtomicMutex":
			if !v.AtLeast(1, 19) && strings.Contains(issues[i].Suggestion, "atomic.Int64") {
				// atomic.Int64, atomic.Bool, atomic.Uint64 (typed atomics) were added
				// in Go 1.19. Older projects must use the function-based API.
				issues[i].Suggestion = "Replace sync.Mutex with atomic.AddInt64 / " +
					"StoreInt64 / LoadInt64 (~27% faster under contention)"
				issues[i].VersionNote = fmt.Sprintf(
					"Go %s: atomic.Int64/Bool/Uint64 (typed atomics) require Go 1.19+. "+
						"Use atomic.AddInt64 / StoreInt64 / LoadInt64 for this version.",
					v.String())
			}
		}
	}
	return issues
}
