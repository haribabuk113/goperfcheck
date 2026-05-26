package main

import (
	"strings"
	"testing"
)

func TestFileLinkFormat(t *testing.T) {
	got := fileLink("/abs/path/to/file.go", "rel/file.go", 10, 5, false)
	want := "rel/file.go:10:5"
	if got != want {
		t.Errorf("fileLink = %q, want %q", got, want)
	}
}

func TestFileLinkColorSameAsPlain(t *testing.T) {
	plain := fileLink("/abs/path/to/file.go", "rel/file.go", 10, 5, false)
	color := fileLink("/abs/path/to/file.go", "rel/file.go", 10, 5, true)
	if plain != color {
		t.Errorf("fileLink color=%q must equal plain=%q — no escape codes expected", color, plain)
	}
}

func TestFileLinkNoEscapeCodes(t *testing.T) {
	for _, color := range []bool{false, true} {
		got := fileLink("/abs/path/to/file.go", "rel/file.go", 10, 5, color)
		if strings.Contains(got, "\x1b") {
			t.Errorf("fileLink(color=%v) must not contain escape codes, got %q", color, got)
		}
	}
}

func TestFileLinkContainsLineAndCol(t *testing.T) {
	got := fileLink("/abs/file.go", "rel/file.go", 42, 7, true)
	if !strings.Contains(got, "rel/file.go:42:7") {
		t.Errorf("fileLink must contain rel:line:col, got %q", got)
	}
}

func TestFileLinkDotSlash(t *testing.T) {
	got := fileLink("/abs/file.go", "./checker.go", 99, 1, true)
	if !strings.Contains(got, "./checker.go:99:1") {
		t.Errorf("fileLink = %q, want to contain ./checker.go:99:1", got)
	}
}
