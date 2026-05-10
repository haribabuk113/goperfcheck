package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/haribabuk113/goperfcheck/checker"
)

// printGoVersionLine prints a one-line version note after a scan result.
// When the version was detected it confirms what was used. When it is unknown
// and the user did not set -go-version, it suggests using the flag so they
// get version-aware advice.
func printGoVersionLine(v checker.GoVersion, source, flagVal string, stdinMode, plain bool) {
	if stdinMode {
		// No go.mod search for stdin; only show if the user set -go-version.
		if !v.Zero() && source == "flag" {
			fmt.Printf("Go: %s (from -go-version flag)\n", v.String())
		}
		return
	}
	if v.Zero() {
		if flagVal == "" {
			if plain {
				fmt.Println("Tip: no go.mod found -- use -go-version=X.Y for version-aware advice")
			} else {
				fmt.Println("Tip: no go.mod found — use -go-version=X.Y for version-aware advice")
			}
		}
		return
	}
	sourceLabel := source
	if source == "flag" {
		sourceLabel = "-go-version flag"
	}
	fmt.Printf("Go: %s (%s)\n", v.String(), sourceLabel)
}

// printAuditReport prints the suppression audit results to stdout.
// Returns true if any stale or expired suppressions were found (caller
// should exit 1 in that case).
func printAuditReport(suppressions []checker.SuppressedIssue, dir string, plain bool) bool {
	symOK := "✓"
	symFile := "📁"
	symSep := "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	if plain {
		symOK = "[OK]"
		symFile = "--"
		symSep = "-------------------------------------------------"
	}

	sort.Slice(suppressions, func(i, j int) bool {
		if suppressions[i].File != suppressions[j].File {
			return suppressions[i].File < suppressions[j].File
		}
		return suppressions[i].Line < suppressions[j].Line
	})

	var nStale, nExpired, nProblematic int
	for _, s := range suppressions {
		if s.Stale {
			nStale++
		}
		if s.Expired {
			nExpired++
		}
		if s.Stale || s.Expired {
			nProblematic++
		}
	}
	total := len(suppressions)

	fmt.Printf("Suppression audit\n%s\n", symSep)

	if total == 0 {
		fmt.Printf("%s No //goperfcheck:ignore comments found.\n", symOK)
		return false
	}

	if nProblematic == 0 {
		fmt.Printf("%s All %d suppression(s) active — none stale or expired.\n", symOK, total)
		return false
	}

	fmt.Printf("Found %d problematic suppression(s) of %d total (%d stale, %d expired).\n",
		nProblematic, total, nStale, nExpired)

	prevFile := ""
	for _, s := range suppressions {
		if !s.Stale && !s.Expired {
			continue
		}
		rel, _ := filepath.Rel(dir, s.File)
		if rel == "" {
			rel = s.File
		}
		if rel != prevFile {
			fmt.Printf("\n%s %s\n", symFile, rel)
			prevFile = rel
		}

		tag := "[STALE]  "
		if s.Expired {
			tag = "[EXPIRED]"
		}

		var suffix string
		if len(s.Checkers) > 0 {
			suffix = " " + strings.Join(s.Checkers, ",")
		}
		if s.Until != "" {
			suffix += " until:" + s.Until
		}

		fmt.Printf("   line %-4d %s  //goperfcheck:ignore%s\n", s.Line, tag, suffix)
	}

	fmt.Printf("\n%s\n", symSep)
	if nStale > 0 {
		fmt.Println("Stale: the suppressed checker no longer fires — safe to remove the comment.")
	}
	if nExpired > 0 {
		fmt.Println("Expired: the until: date has passed — revisit and remove or renew the suppression.")
	}
	return true
}
