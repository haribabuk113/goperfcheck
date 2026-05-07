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
func checkFileConcurrent(path string, checkers []checker.Checker) []checker.Issue {
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, path, nil, parser.AllErrors|parser.ParseComments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error %s: %v\n", path, err)
		return nil
	}
	var issues []checker.Issue
	for _, c := range checkers {
		issues = append(issues, c.Check(fset, astFile)...)
	}
	return checker.FilterSuppressed(fset, astFile, issues)
}

// scanFiles checks all paths in parallel using numWorkers goroutines.
// skipTests skips *_test.go files when true.
// Results arrive in non-deterministic order; callers are expected to sort.
func scanFiles(paths []string, checkers []checker.Checker, skipTests bool, numWorkers int) []checker.Issue {
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
				if batch := checkFileConcurrent(path, checkers); len(batch) > 0 {
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
