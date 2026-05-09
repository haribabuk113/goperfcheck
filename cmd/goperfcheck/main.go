// goperfcheck is a static analysis tool that verifies Go code against the
// performance guidelines from https://goperf.dev.
//
// It scans a repository and reports violations of best practices in:
//   - Memory allocation and preallocation
//   - Object pooling patterns
//   - Struct field alignment
//   - Interface boxing
//   - Zero-copy techniques
//   - Goroutine worker pools
//   - Context management
//   - Buffered I/O
//   - Atomic operations vs mutexes
//   - Lazy initialization
//   - Stack allocations
//   - Batching operations
//
// Usage:
//
//	goperfcheck [-dir <path>] [-file <file.go>] [-checker <name>] [-severity INFO|WARN|ERROR] [-git-staged] [-output report.md|report.sarif] [-fix]
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/haribabuk113/goperfcheck/checker"
)

const version = "0.1.0"

func main() {
	dir := flag.String("dir", ".", "root directory to scan (default: current directory)")
	file := flag.String("file", "", "check a single Go file instead of scanning a directory")
	checkerName := flag.String("checker", "", "run only the named checker (e.g. mem-prealloc); use -list-checkers to see all names")
	group := flag.String("group", "", "run only checkers in a theme group: memory | concurrency | io")
	skipVendor := flag.Bool("skip-vendor", true, "skip the vendor/ directory")
	skipTests := flag.Bool("skip-tests", false, "skip *_test.go files")
	severity := flag.String("severity", "INFO", "minimum severity to report: INFO | WARN | ERROR")
	gitStaged := flag.Bool("git-staged", false, "only check Go files staged for the next git commit")
	output := flag.String("output", "", "write report to a file: .md for Markdown, .sarif for SARIF 2.1.0 (GitHub Code Scanning)")
	format := flag.String("format", "text", "output format: text | json")
	workers := flag.Int("workers", defaultWorkers(), "number of parallel workers for file scanning")
	fix := flag.Bool("fix", false, "auto-apply fixable suggestions in place (modifies source files)")
	noColor := flag.Bool("no-color", false, "disable emoji and Unicode box-drawing in output (also respects NO_COLOR env var)")
	stdin := flag.Bool("stdin", false, "read Go source from stdin instead of a file or directory")
	showVersion := flag.Bool("version", false, "print version and exit")

	// Load .goperfcheck config before flag.Parse so CLI flags take precedence.
	cfg, cfgPath, cfgErr := loadConfig()
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", cfgErr)
		os.Exit(2)
	}
	for k, v := range cfg {
		if setErr := flag.Set(k, v); setErr != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: unknown or invalid setting %q = %q\n", cfgPath, k, v)
		}
	}

	flag.Parse()

	_, noColorEnv := os.LookupEnv("NO_COLOR")
	plain := *noColor || noColorEnv

	symOK := "✓"
	symFile := "📁"
	symWarn := "⚠ "
	symHint := "💡"
	symBench := "📊"
	symSep := "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	if plain {
		symOK = "[OK]"
		symFile = "--"
		symWarn = "!"
		symHint = "hint:"
		symBench = "bench:"
		symSep = "-------------------------------------------------"
	}

	if *showVersion {
		fmt.Printf("goperfcheck v%s\n", version)
		return
	}

	if *stdin && *fix {
		fmt.Fprintf(os.Stderr, "error: -stdin and -fix cannot be used together\n")
		os.Exit(1)
	}
	if *stdin && *file != "" {
		fmt.Fprintf(os.Stderr, "error: -stdin and -file cannot be used together\n")
		os.Exit(1)
	}
	if *stdin && *gitStaged {
		fmt.Fprintf(os.Stderr, "error: -stdin and -git-staged cannot be used together\n")
		os.Exit(1)
	}

	if *checkerName != "" && *group != "" {
		fmt.Fprintf(os.Stderr, "error: -checker and -group cannot be used together\n")
		os.Exit(1)
	}

	minSev := parseSeverity(*severity)

	allCheckers := checker.AllCheckers()

	if *checkerName != "" {
		var matched []checker.Checker
		for _, c := range allCheckers {
			if strings.EqualFold(c.Name(), *checkerName) {
				matched = append(matched, c)
				break
			}
		}
		if len(matched) == 0 {
			fmt.Fprintf(os.Stderr, "unknown checker %q — valid names:\n", *checkerName)
			for _, c := range allCheckers {
				fmt.Fprintf(os.Stderr, "  %s\n", c.Name())
			}
			os.Exit(1)
		}
		allCheckers = matched
	}

	if *group != "" {
		matched, ok := checker.CheckersForGroup(*group, allCheckers)
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown group %q — valid groups:\n", *group)
			for _, g := range checker.SortedGroupNames() {
				checkerNames := checker.Groups[g]
				fmt.Fprintf(os.Stderr, "  %-14s %s\n", g, strings.Join(checkerNames, ", "))
			}
			os.Exit(1)
		}
		allCheckers = matched
	}

	var allIssues []checker.Issue

	if *stdin {
		src, readErr := io.ReadAll(os.Stdin)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "stdin read error: %v\n", readErr)
			os.Exit(2)
		}
		fset := token.NewFileSet()
		astFile, parseErr := parser.ParseFile(fset, "<stdin>", src, parser.AllErrors|parser.ParseComments)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "parse error: %v\n", parseErr)
			os.Exit(2)
		}
		var raw []checker.Issue
		for _, c := range allCheckers {
			raw = append(raw, c.Check(fset, astFile)...)
		}
		raw = checker.FilterSuppressed(fset, astFile, raw)
		for _, issue := range raw {
			if severityLevel(issue.Severity) >= severityLevel(minSev) {
				allIssues = append(allIssues, issue)
			}
		}
	} else {
		// Collect file paths first, then scan in parallel.
		var paths []string

		if *file != "" {
			if !strings.HasSuffix(*file, ".go") {
				fmt.Fprintf(os.Stderr, "error: -file must point to a .go file\n")
				os.Exit(1)
			}
			abs, absErr := filepath.Abs(*file)
			if absErr != nil {
				fmt.Fprintf(os.Stderr, "abs error: %v\n", absErr)
				os.Exit(1)
			}
			paths = []string{abs}
		} else if *gitStaged {
			root, absErr := filepath.Abs(*dir)
			if absErr != nil {
				fmt.Fprintf(os.Stderr, "abs error: %v\n", absErr)
				os.Exit(1)
			}
			cmd := exec.Command("git", "diff", "--name-only", "--cached", "--diff-filter=d")
			cmd.Dir = root
			out, cmdErr := cmd.Output()
			if cmdErr != nil {
				fmt.Fprintf(os.Stderr, "git error: %v\n", cmdErr)
				os.Exit(1)
			}
			lines := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
			for _, line := range lines {
				name := strings.TrimSpace(string(line))
				if name == "" || !strings.HasSuffix(name, ".go") {
					continue
				}
				if strings.Contains(filepath.Clean(name), "..") {
					continue
				}
				paths = append(paths, filepath.Join(root, name))
			}
		} else {
			err := filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					base := filepath.Base(path)
					if *skipVendor && base == "vendor" {
						return filepath.SkipDir
					}
					if strings.HasPrefix(base, ".") {
						return filepath.SkipDir
					}
					return nil
				}
				if !strings.HasSuffix(path, ".go") {
					return nil
				}
				paths = append(paths, path)
				return nil
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "walk error: %v\n", err)
				os.Exit(1)
			}
		}

		numW := *workers
		if numW < 1 {
			numW = 1
		}
		raw := scanFiles(paths, allCheckers, *skipTests, numW)
		for _, issue := range raw {
			if severityLevel(issue.Severity) >= severityLevel(minSev) {
				allIssues = append(allIssues, issue)
			}
		}
	}

	// Deduplicate issues (can occur with nested loops)
	allIssues = checker.DedupeIssues(allIssues)

	// Sort by file, then line
	sort.Slice(allIssues, func(i, j int) bool {
		if allIssues[i].File != allIssues[j].File {
			return allIssues[i].File < allIssues[j].File
		}
		return allIssues[i].Line < allIssues[j].Line
	})

	if *fix {
		n, fixErr := applyFixes(allIssues)
		if fixErr != nil {
			fmt.Fprintf(os.Stderr, "fix error: %v\n", fixErr)
		}
		if n > 0 {
			fmt.Printf("Applied %d fix(es) — re-run without -fix to see remaining issues\n", n)
		} else {
			fmt.Printf("No auto-fixable issues found\n")
		}
		return
	}

	if *output != "" {
		var writeErr error
		if strings.HasSuffix(strings.ToLower(*output), ".sarif") {
			writeErr = writeSARIFReport(*output, allIssues, *dir)
		} else {
			writeErr = writeMarkdownReport(*output, allIssues, *dir)
		}
		if writeErr != nil {
			fmt.Fprintf(os.Stderr, "report error: %v\n", writeErr)
			os.Exit(2)
		}
		if len(allIssues) == 0 {
			fmt.Printf("%s No issues found — report written to %s\n", symOK, *output)
		} else {
			fmt.Printf("Report written to %s (%d issue(s))\n", *output, len(allIssues))
			os.Exit(1)
		}
		return
	}

	if strings.ToLower(*format) == "json" {
		printJSON(allIssues)
		if len(allIssues) > 0 {
			os.Exit(1)
		}
		return
	}

	if len(allIssues) == 0 {
		switch {
		case *stdin:
			fmt.Printf("%s No performance issues found in <stdin>\n", symOK)
		case *file != "":
			fmt.Printf("%s No performance issues found in %s\n", symOK, *file)
		case *gitStaged:
			fmt.Printf("%s No performance issues found in staged files\n", symOK)
		default:
			fmt.Printf("%s No performance issues found in %s\n", symOK, *dir)
		}
		return
	}

	// Pre-count issues per relative path so the file header can show the total.
	countPerFile := make(map[string]int, len(allIssues))
	for _, issue := range allIssues {
		rel, _ := filepath.Rel(*dir, issue.File)
		if rel == "" {
			rel = issue.File
		}
		countPerFile[rel]++
	}

	// Print results grouped by file
	prevFile := ""
	for _, issue := range allIssues {
		rel, _ := filepath.Rel(*dir, issue.File)
		if rel == "" {
			rel = issue.File
		}
		if rel != prevFile {
			fmt.Printf("\n%s %s  (%d issue(s))\n", symFile, rel, countPerFile[rel])
			prevFile = rel
		}
		fmt.Printf("   [%s] %s:%d:%d\n", issue.Severity, issue.Checker, issue.Line, issue.Column)
		fmt.Printf("   %s %s\n", symWarn, issue.Message)
		if issue.Suggestion != "" {
			fmt.Printf("   %s %s\n", symHint, issue.Suggestion)
		}
		if issue.Benchmark != "" {
			fmt.Printf("   %s %s\n", symBench, issue.Benchmark)
		}
	}

	fmt.Printf("\n%s\n", symSep)
	fmt.Printf("Found %d performance issue(s)\n", len(allIssues))

	// Exit with error code if any issues found
	os.Exit(1)
}

func printJSON(issues []checker.Issue) {
	if issues == nil {
		issues = []checker.Issue{}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(issues); err != nil {
		fmt.Fprintf(os.Stderr, "json encode error: %v\n", err)
		os.Exit(2)
	}
}

func parseSeverity(s string) checker.Severity {
	switch strings.ToUpper(s) {
	case "ERROR":
		return checker.SeverityError
	case "WARN", "WARNING":
		return checker.SeverityWarning
	default:
		return checker.SeverityInfo
	}
}

func severityLevel(s checker.Severity) int {
	switch s {
	case checker.SeverityError:
		return 3
	case checker.SeverityWarning:
		return 2
	default:
		return 1
	}
}
