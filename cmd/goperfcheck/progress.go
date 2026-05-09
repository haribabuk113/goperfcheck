package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/haribabuk113/goperfcheck/checker"
)

// progressThreshold is the minimum number of files before a progress indicator
// is shown on stderr. Below this count scanning finishes fast enough that
// printing a progress line would be more noise than signal.
const progressThreshold = 50

// progressLineWidth is the number of spaces used to erase the progress line.
// Wide enough to cover the longest plausible progress string.
const progressLineWidth = 60

// runScanWithProgress calls scanFiles while displaying a live progress
// indicator on stderr when:
//   - stderr is an interactive terminal, AND
//   - len(paths) >= progressThreshold
//
// The indicator is erased before the function returns, so subsequent output
// (results, summary) is not affected. When the conditions above are not met
// the function delegates directly to scanFiles with no overhead.
func runScanWithProgress(
	paths []string,
	checkers []checker.Checker,
	skipTests, skipGenerated bool,
	numWorkers int,
	cc cacheConfig,
	progress *atomic.Int64,
) []checker.Issue {
	total := len(paths)
	showProgress := total >= progressThreshold && isStderrTerminal()

	if !showProgress {
		return scanFiles(paths, checkers, skipTests, skipGenerated, numWorkers, cc, progress)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				n := progress.Load()
				fmt.Fprintf(os.Stderr, "\rScanning... (%d/%d files)", n, total)
			case <-stop:
				return
			}
		}
	}()

	issues := scanFiles(paths, checkers, skipTests, skipGenerated, numWorkers, cc, progress)

	close(stop)
	wg.Wait()
	// Erase the progress line so results print on a clean line.
	fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", progressLineWidth))

	return issues
}
