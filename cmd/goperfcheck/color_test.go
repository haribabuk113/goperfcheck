package main

import (
	"strings"
	"testing"
)

func TestFileLinkPlainMode(t *testing.T) {
	got := fileLink("/abs/path/to/file.go", "rel/file.go", 10, 5, false)
	want := "rel/file.go:10:5"
	if got != want {
		t.Errorf("fileLink plain = %q, want %q", got, want)
	}
}

func TestFileLinkPlainModeNoEscapes(t *testing.T) {
	got := fileLink("/abs/path/to/file.go", "rel/file.go", 10, 5, false)
	if strings.Contains(got, "\x1b") {
		t.Errorf("fileLink plain mode must not contain escape codes, got %q", got)
	}
}

func TestFileLinkColorModeContainsOSC8(t *testing.T) {
	got := fileLink("/abs/path/to/file.go", "rel/file.go", 42, 3, true)
	if !strings.Contains(got, "\x1b]8;;") {
		t.Errorf("fileLink color mode must contain OSC 8 open sequence, got %q", got)
	}
	if !strings.Contains(got, "\x1b]8;;\x1b\\") {
		t.Errorf("fileLink color mode must contain OSC 8 close sequence, got %q", got)
	}
}

func TestFileLinkColorModeContainsFileURI(t *testing.T) {
	got := fileLink("/home/user/project/main.go", "main.go", 1, 1, true)
	if !strings.Contains(got, "file:///home/user/project/main.go") {
		t.Errorf("fileLink color mode must embed file:// URI with abs path, got %q", got)
	}
}

func TestFileLinkColorModeDisplayText(t *testing.T) {
	got := fileLink("/abs/file.go", "rel/file.go", 10, 5, true)
	if !strings.Contains(got, "rel/file.go:10:5") {
		t.Errorf("fileLink color mode must contain display text rel:line:col, got %q", got)
	}
}

func TestFileLinkColorModeDotSlash(t *testing.T) {
	got := fileLink("/abs/file.go", "./checker.go", 99, 1, true)
	if !strings.Contains(got, "./checker.go:99:1") {
		t.Errorf("fileLink color mode display text = %q, want to contain ./checker.go:99:1", got)
	}
}
