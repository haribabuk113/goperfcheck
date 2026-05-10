package checker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseGoVersion(t *testing.T) {
	tests := []struct {
		in        string
		wantMajor int
		wantMinor int
		wantOK    bool
	}{
		{"1.22", 1, 22, true},
		{"1.22.0", 1, 22, true},
		{"1.22.1", 1, 22, true},
		{"1.21rc3", 1, 21, true},
		{"go1.22", 1, 22, true},
		{"GO1.22", 1, 22, true},
		{"1.17", 1, 17, true},
		{"1.19", 1, 19, true},
		{"1.20", 1, 20, true},
		{"1.26.1", 1, 26, true},
		{"", 0, 0, false},
		{"invalid", 0, 0, false},
		{"abc.def", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			v, ok := ParseGoVersion(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("ParseGoVersion(%q) ok=%v, want %v", tt.in, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if v.Major != tt.wantMajor || v.Minor != tt.wantMinor {
				t.Errorf("ParseGoVersion(%q) = %d.%d, want %d.%d",
					tt.in, v.Major, v.Minor, tt.wantMajor, tt.wantMinor)
			}
		})
	}
}

func TestGoVersionAtLeast(t *testing.T) {
	tests := []struct {
		v          GoVersion
		major, min int
		want       bool
	}{
		{GoVersion{1, 22}, 1, 22, true},
		{GoVersion{1, 22}, 1, 21, true},
		{GoVersion{1, 22}, 1, 23, false},
		{GoVersion{1, 17}, 1, 22, false},
		{GoVersion{1, 22}, 1, 19, true},
		{GoVersion{2, 0}, 1, 22, true},  // major version bump
		{GoVersion{1, 22}, 2, 0, false}, // future major
	}
	for _, tt := range tests {
		got := tt.v.AtLeast(tt.major, tt.min)
		if got != tt.want {
			t.Errorf("GoVersion{%d,%d}.AtLeast(%d,%d) = %v, want %v",
				tt.v.Major, tt.v.Minor, tt.major, tt.min, got, tt.want)
		}
	}
}

func TestReadModGoVersion(t *testing.T) {
	dir := t.TempDir()

	// Write a go.mod with a go directive.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(
		"module example.com/foo\n\ngo 1.22\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}

	v, ok := ReadModGoVersion(dir)
	if !ok {
		t.Fatal("ReadModGoVersion: expected ok=true, got false")
	}
	if v.Major != 1 || v.Minor != 22 {
		t.Errorf("ReadModGoVersion: got %s, want 1.22", v.String())
	}

	// Walk-up: reading from a subdirectory should still find the root go.mod.
	sub := filepath.Join(dir, "pkg", "handler")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	v2, ok2 := ReadModGoVersion(sub)
	if !ok2 {
		t.Fatal("ReadModGoVersion from subdirectory: expected ok=true")
	}
	if v2.Major != 1 || v2.Minor != 22 {
		t.Errorf("ReadModGoVersion walk-up: got %s, want 1.22", v2.String())
	}
}

func TestReadModGoVersion_NoMod(t *testing.T) {
	dir := t.TempDir()
	_, ok := ReadModGoVersion(dir)
	if ok {
		t.Error("expected ok=false when no go.mod present")
	}
}

func TestApplyGoVersion_GoroutinePool(t *testing.T) {
	base := Issue{Checker: "GoroutinePool", Message: "unbounded goroutines"}

	// < 1.22: no VersionNote
	issues := ApplyGoVersion([]Issue{base}, GoVersion{1, 21})
	if issues[0].VersionNote != "" {
		t.Errorf("Go 1.21: expected no VersionNote, got %q", issues[0].VersionNote)
	}

	// >= 1.22: VersionNote added
	issues = ApplyGoVersion([]Issue{base}, GoVersion{1, 22})
	if issues[0].VersionNote == "" {
		t.Error("Go 1.22: expected VersionNote, got empty string")
	}
}

func TestApplyGoVersion_StackAlloc(t *testing.T) {
	base := Issue{Checker: "StackAlloc", Message: "new(int) on heap"}

	// < 1.17: no VersionNote
	issues := ApplyGoVersion([]Issue{base}, GoVersion{1, 16})
	if issues[0].VersionNote != "" {
		t.Errorf("Go 1.16: expected no VersionNote, got %q", issues[0].VersionNote)
	}

	// >= 1.17: VersionNote added
	issues = ApplyGoVersion([]Issue{base}, GoVersion{1, 17})
	if issues[0].VersionNote == "" {
		t.Error("Go 1.17: expected VersionNote, got empty string")
	}
}

func TestApplyGoVersion_ZeroCopy(t *testing.T) {
	base := Issue{Checker: "ZeroCopy", Message: "unnecessary copy"}

	// >= 1.20: no VersionNote (bytes.Clone available)
	issues := ApplyGoVersion([]Issue{base}, GoVersion{1, 20})
	if issues[0].VersionNote != "" {
		t.Errorf("Go 1.20: expected no VersionNote, got %q", issues[0].VersionNote)
	}

	// < 1.20: VersionNote added (bytes.Clone unavailable)
	issues = ApplyGoVersion([]Issue{base}, GoVersion{1, 19})
	if issues[0].VersionNote == "" {
		t.Error("Go 1.19: expected VersionNote about bytes.Clone, got empty string")
	}
}

func TestApplyGoVersion_AtomicMutex(t *testing.T) {
	structSuggestion := "Replace sync.Mutex with atomic.Int64, atomic.Bool, or atomic.Uint64 (27% faster under contention)"
	base := Issue{Checker: "AtomicMutex", Suggestion: structSuggestion}

	// >= 1.19: suggestion unchanged, no VersionNote
	issues := ApplyGoVersion([]Issue{base}, GoVersion{1, 19})
	if issues[0].VersionNote != "" {
		t.Errorf("Go 1.19: expected no VersionNote, got %q", issues[0].VersionNote)
	}
	if issues[0].Suggestion != structSuggestion {
		t.Errorf("Go 1.19: suggestion should be unchanged")
	}

	// < 1.19: suggestion rewritten, VersionNote added
	issues = ApplyGoVersion([]Issue{base}, GoVersion{1, 18})
	if issues[0].VersionNote == "" {
		t.Error("Go 1.18: expected VersionNote, got empty string")
	}
	if issues[0].Suggestion == structSuggestion {
		t.Error("Go 1.18: suggestion should have been rewritten to use function-based atomics")
	}
	// Verify the rewritten suggestion does NOT reference atomic.Int64
	if contains(issues[0].Suggestion, "atomic.Int64") && !contains(issues[0].Suggestion, "require Go 1.19") {
		t.Errorf("Go 1.18: rewritten suggestion still mentions atomic.Int64 without caveat: %q", issues[0].Suggestion)
	}
}

func TestApplyGoVersion_ZeroVersion(t *testing.T) {
	base := Issue{Checker: "GoroutinePool", Message: "unbounded"}
	issues := ApplyGoVersion([]Issue{base}, GoVersion{})
	if issues[0].VersionNote != "" {
		t.Error("zero version: no adjustments should be made")
	}
}
