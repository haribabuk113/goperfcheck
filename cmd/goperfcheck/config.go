package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const configFileName = ".goperfcheck"

// findConfig walks up from the current working directory until it finds a
// .goperfcheck file or reaches the filesystem root.
func findConfig() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		candidate := filepath.Join(dir, configFileName)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir { // filesystem root
			return "", false
		}
		dir = parent
	}
}

// loadConfig finds and parses the nearest .goperfcheck file, returning a map
// of flag-name → value. Lines beginning with # are comments; trailing inline
// # comments are also stripped. Returns an empty map when no file is present.
func loadConfig() (cfg map[string]string, path string, err error) {
	path, found := findConfig()
	if !found {
		return map[string]string{}, "", nil
	}

	f, openErr := os.Open(path)
	if openErr != nil {
		return nil, path, openErr
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	cfg = make(map[string]string)
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return nil, path, fmt.Errorf("%s:%d: expected key = value, got %q", path, lineNum, line)
		}
		// Strip inline comments.
		if idx := strings.Index(val, "#"); idx >= 0 {
			val = val[:idx]
		}
		cfg[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	return cfg, path, scanner.Err()
}
