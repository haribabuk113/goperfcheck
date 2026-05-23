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
