package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/haribabuk113/goperfcheck/checker"
)

// defaultWorkers returns the number of logical CPUs as the default worker count.
func defaultWorkers() int {
	if n := runtime.NumCPU(); n > 1 {
		return n
	}
	return 1
}

// checkFileConcurrent parses and checks a single file. It creates its own
// token.FileSet so it is safe to call from multiple goroutines simultaneously.
// When skipGenerated is true, files carrying a "// Code generated" header are
// silently skipped. When cc.enabled is true, results are looked up from and
// written to the on-disk cache keyed by file content hash + config hash.
func checkFileConcurrent(path string, checkers []checker.Checker, skipGenerated bool, cc cacheConfig) []checker.Issue {
	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read error %s: %v\n", path, err)
		return nil
	}

	// Cache lookup — skip parsing entirely on a hit.
	var entryName string
	if cc.enabled {
		fh := fileContentHash(content)
		entryName = cacheEntryName(fh, cc.cfgHash)
		if cached, ok := readCacheEntry(cc.dir, entryName); ok {
			return cached
		}
	}

	fset := token.NewFileSet()
	astFile, parseErr := parser.ParseFile(fset, path, content, parser.AllErrors|parser.ParseComments)
	if parseErr != nil {
		fmt.Fprintf(os.Stderr, "parse error %s: %v\n", path, parseErr)
		return nil
	}
	if skipGenerated && isGeneratedFile(astFile) {
		if cc.enabled {
			writeCacheEntry(cc.dir, entryName, nil)
		}
		return nil
	}

	var issues []checker.Issue
	for _, c := range checkers {
		issues = append(issues, c.Check(fset, astFile)...)
	}
	issues = checker.FilterSuppressed(fset, astFile, issues)

	if cc.enabled {
		writeCacheEntry(cc.dir, entryName, issues)
	}
	return issues
}

// scanFiles checks all paths in parallel using numWorkers goroutines.
// skipTests skips *_test.go files; skipGenerated skips files with a
// "// Code generated" header. Results arrive in non-deterministic order;
// callers are expected to sort.
func scanFiles(paths []string, checkers []checker.Checker, skipTests, skipGenerated bool, numWorkers int, cc cacheConfig) []checker.Issue {
	if len(paths) == 0 {
		return nil
	}

	jobs := make(chan string)
	results := make(chan []checker.Issue)

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				if skipTests && strings.HasSuffix(path, "_test.go") {
					continue
				}
				if batch := checkFileConcurrent(path, checkers, skipGenerated, cc); len(batch) > 0 {
					results <- batch
				}
			}
		}()
	}

	go func() {
		for _, p := range paths {
			jobs <- p
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	var all []checker.Issue
	for batch := range results {
		all = append(all, batch...)
	}
	return all
}
