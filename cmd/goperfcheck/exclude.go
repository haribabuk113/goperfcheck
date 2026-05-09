package main

import (
	"go/ast"
	"path/filepath"
	"strings"
)

// parseExcludePatterns splits a comma-separated -exclude value into trimmed,
// non-empty patterns.
func parseExcludePatterns(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// dirExcluded reports whether a directory should be skipped. Patterns ending
// with "/" are treated as directory names; the base name is compared directly.
func dirExcluded(base string, patterns []string) bool {
	for _, pat := range patterns {
		if strings.HasSuffix(pat, "/") && strings.TrimSuffix(pat, "/") == base {
			return true
		}
	}
	return false
}

// fileExcluded reports whether a file should be skipped. Patterns not ending
// in "/" are matched against the file's base name using filepath.Match.
func fileExcluded(path string, patterns []string) bool {
	base := filepath.Base(path)
	for _, pat := range patterns {
		if strings.HasSuffix(pat, "/") {
			continue // directory patterns are enforced during the walk
		}
		if matched, _ := filepath.Match(pat, base); matched {
			return true
		}
	}
	return false
}

// isGeneratedFile reports whether the parsed Go file carries the standard
// "// Code generated" marker (https://go.dev/s/generatedcode). The comment
// may appear anywhere in the file; the AST already has all comments parsed.
func isGeneratedFile(file *ast.File) bool {
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			if strings.HasPrefix(c.Text, "// Code generated") {
				return true
			}
		}
	}
	return false
}
