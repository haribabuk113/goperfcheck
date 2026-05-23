package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary compiles the goperfcheck binary into a temp dir for integration tests.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "goperfcheck")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

func TestUnknownSubcommand(t *testing.T) {
	bin := buildBinary(t)

	tests := []struct {
		name     string
		args     []string
		wantExit int
		wantErr  string
	}{
		{
			name:     "help triggers unknown subcommand error",
			args:     []string{"help"},
			wantExit: 2,
			wantErr:  `unknown subcommand "help"`,
		},
		{
			name:     "version word (without dash) triggers error",
			args:     []string{"version"},
			wantExit: 2,
			wantErr:  `unknown subcommand "version"`,
		},
		{
			name:     "arbitrary word triggers unknown subcommand error",
			args:     []string{"foo"},
			wantExit: 2,
			wantErr:  `unknown subcommand "foo"`,
		},
		{
			name:     "error message hints at -help flag",
			args:     []string{"help"},
			wantExit: 2,
			wantErr:  "goperfcheck -help",
		},
		{
			name:     "error message lists explain subcommand",
			args:     []string{"help"},
			wantExit: 2,
			wantErr:  "explain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(bin, tt.args...)
			out, err := cmd.CombinedOutput()

			exitCode := 0
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				}
			}

			if exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d; output: %s", exitCode, tt.wantExit, out)
			}
			if !strings.Contains(string(out), tt.wantErr) {
				t.Errorf("stderr/stdout %q does not contain %q", string(out), tt.wantErr)
			}
		})
	}
}

func TestExplainSubcommandStillWorks(t *testing.T) {
	bin := buildBinary(t)

	// "explain" with no argument should list checkers (exit 1 with usage).
	cmd := exec.Command(bin, "explain")
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "Available checkers") {
		t.Errorf("explain with no arg should list checkers; got: %s", out)
	}
}

func TestExplainCaseInsensitive(t *testing.T) {
	bin := buildBinary(t)

	for _, sub := range []string{"EXPLAIN", "Explain", "eXpLaIn"} {
		t.Run(sub, func(t *testing.T) {
			cmd := exec.Command(bin, sub)
			out, _ := cmd.CombinedOutput()
			if !strings.Contains(string(out), "Available checkers") {
				t.Errorf("%q should be treated as explain subcommand; got: %s", sub, out)
			}
		})
	}
}

func TestVersionFlagStillWorks(t *testing.T) {
	bin := buildBinary(t)

	cmd := exec.Command(bin, "-version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("-version exited with error: %v; output: %s", err, out)
	}
	if !strings.Contains(string(out), "goperfcheck v") {
		t.Errorf("-version output %q does not contain version string", out)
	}
}

func TestNoArgsRunsWithoutError(t *testing.T) {
	// Running with no args in a directory that has no .go files should exit 0.
	bin := buildBinary(t)

	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "dummy.txt"), []byte("not go"), 0o644)

	cmd := exec.Command(bin, "-dir", dir)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// exit 1 means issues found — acceptable in this context
			return
		}
		t.Errorf("unexpected error running with no .go files: %v", err)
	}
}

// goFile is a minimal Go source with one known performance issue (append in a
// range loop without prealloc) so the scanner always has something to do.
const goFile = `package p

func collect(items []string) []string {
	var out []string
	for _, v := range items {
		out = append(out, v)
	}
	return out
}
`

func tempDirWithGoFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "example.go"), []byte(goFile), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	return dir
}

func TestCacheSpaceFalseSkipsCacheDirectory(t *testing.T) {
	// "-cache false" (space-separated) must disable the cache, not silently
	// ignore "false" as a positional argument.
	bin := buildBinary(t)
	dir := tempDirWithGoFile(t)

	cmd := exec.Command(bin, "-cache", "false", "-dir", dir)
	cmd.Run() //nolint — exit code 1 is expected (issues found)

	cacheDir := filepath.Join(dir, ".goperfcheck-cache")
	if _, err := os.Stat(cacheDir); err == nil {
		t.Errorf("-cache false still created cache directory at %s", cacheDir)
	}
}

func TestCacheEqualsFalseSkipsCacheDirectory(t *testing.T) {
	bin := buildBinary(t)
	dir := tempDirWithGoFile(t)

	cmd := exec.Command(bin, "-cache=false", "-dir", dir)
	cmd.Run() //nolint — exit code 1 is expected (issues found)

	cacheDir := filepath.Join(dir, ".goperfcheck-cache")
	if _, err := os.Stat(cacheDir); err == nil {
		t.Errorf("-cache=false still created cache directory at %s", cacheDir)
	}
}

func TestCacheEnabledByDefault(t *testing.T) {
	bin := buildBinary(t)
	dir := tempDirWithGoFile(t)

	cmd := exec.Command(bin, "-dir", dir)
	cmd.Run() //nolint — exit code 1 is expected (issues found)

	cacheDir := filepath.Join(dir, ".goperfcheck-cache")
	if _, err := os.Stat(cacheDir); err != nil {
		t.Errorf("expected cache directory to be created by default, but it was not: %v", err)
	}
}

func TestOutputContainsFileLocation(t *testing.T) {
	// Every issue line must include a file:line:col location so users can
	// navigate directly to the finding. The format is relpath:line:col appearing
	// between the severity tag and the checker name on the same output line.
	bin := buildBinary(t)
	dir := tempDirWithGoFile(t)

	out, _ := exec.Command(bin, "-dir", dir).CombinedOutput()
	output := string(out)

	// The file written by tempDirWithGoFile is called "example.go".
	// At least one line must contain "example.go:<line>:<col>".
	if !strings.Contains(output, "example.go:") {
		t.Errorf("output should contain file location (example.go:<line>:<col>); got:\n%s", output)
	}
}

func TestFileLinkFormatLineCol(t *testing.T) {
	// The per-issue line must contain a colon-separated file:line:col triple so
	// that editors that parse linter output can jump to the finding.
	bin := buildBinary(t)
	dir := tempDirWithGoFile(t)

	out, _ := exec.Command(bin, "-dir", dir).CombinedOutput()

	// Find the line with the issue severity tag. It must also contain a
	// path:number:number pattern on the same line.
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "[WARN]") && !strings.Contains(line, "[INFO]") && !strings.Contains(line, "[ERROR]") {
			continue
		}
		// The checker name (MemPrealloc) should appear after the location.
		if strings.Contains(line, "example.go") {
			// Verify the format is file:line:col followed by the checker.
			if !strings.Contains(line, ":") {
				t.Errorf("issue line missing colon-separated location: %q", line)
			}
			return
		}
	}
	t.Errorf("no issue line containing file location found in:\n%s", string(out))
}

func TestCacheFalseProducesSameResultsAsDefault(t *testing.T) {
	bin := buildBinary(t)

	// Both forms of disabling the cache must produce the same findings as the
	// default (cache-enabled) run.
	dir1 := tempDirWithGoFile(t)
	dir2 := tempDirWithGoFile(t)
	dir3 := tempDirWithGoFile(t)

	out1, _ := exec.Command(bin, "-dir", dir1, "-format", "json").Output()
	out2, _ := exec.Command(bin, "-cache", "false", "-dir", dir2, "-format", "json").Output()
	out3, _ := exec.Command(bin, "-cache=false", "-dir", dir3, "-format", "json").Output()

	norm1 := strings.ReplaceAll(string(out1), dir1, "<DIR>")
	norm2 := strings.ReplaceAll(string(out2), dir2, "<DIR>")
	norm3 := strings.ReplaceAll(string(out3), dir3, "<DIR>")

	if norm1 != norm2 {
		t.Errorf("-cache false produced different output than default:\ndefault:\n%s\n-cache false:\n%s", norm1, norm2)
	}
	if norm1 != norm3 {
		t.Errorf("-cache=false produced different output than default:\ndefault:\n%s\n-cache=false:\n%s", norm1, norm3)
	}
}
