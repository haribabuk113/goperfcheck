package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/haribabuk113/goperfcheck/checker"
)

const cacheDirName = ".goperfcheck-cache"

// cacheConfig carries everything the scanner needs to read/write the cache.
// When enabled is false the cache is bypassed entirely.
type cacheConfig struct {
	enabled bool
	dir     string // absolute path to .goperfcheck-cache/
	cfgHash string // 8-char hex hash of version + active checker names
}

// buildCacheConfig constructs a cacheConfig. When cache is false it returns a
// disabled config so call-sites do not need special-case logic.
func buildCacheConfig(cache bool, dir string, checkers []checker.Checker) cacheConfig {
	if !cache {
		return cacheConfig{}
	}
	return cacheConfig{enabled: true, dir: dir, cfgHash: computeConfigHash(checkers)}
}

// computeConfigHash hashes the tool version and sorted checker names into an
// 8-character hex string. Changing -checker/-group or upgrading the binary
// automatically invalidates all existing cache entries.
func computeConfigHash(checkers []checker.Checker) string {
	names := make([]string, len(checkers))
	for i, c := range checkers {
		names[i] = c.Name()
	}
	sort.Strings(names)
	h := sha256.Sum256([]byte(version + "\x00" + strings.Join(names, ",")))
	return hex.EncodeToString(h[:4]) // 8 hex chars
}

// fileContentHash returns the full 64-char hex SHA-256 of content.
func fileContentHash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

// cacheEntryName builds the filename: first 32 chars of the file hash +
// underscore + 8-char config hash + ".json".
func cacheEntryName(fileHash, cfgHash string) string {
	return fileHash[:32] + "_" + cfgHash + ".json"
}

// readCacheEntry returns cached issues and true on a hit; nil, false on any miss.
func readCacheEntry(dir, name string) ([]checker.Issue, bool) {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, false
	}
	var issues []checker.Issue
	if err := json.Unmarshal(data, &issues); err != nil {
		return nil, false
	}
	return issues, true
}

// writeCacheEntry atomically writes issues to dir/name via a temp-file rename.
// Errors are silently ignored — cache misses are always safe.
func writeCacheEntry(dir, name string, issues []checker.Issue) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	if issues == nil {
		issues = []checker.Issue{}
	}
	data, err := json.Marshal(issues)
	if err != nil {
		return
	}
	tmp := filepath.Join(dir, name+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, filepath.Join(dir, name))
}
