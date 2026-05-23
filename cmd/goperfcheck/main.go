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
	"sync/atomic"
	"time"

	"github.com/haribabuk113/goperfcheck/checker"
)

const version = "0.2.0"

func main() {
	dir := flag.String("dir", ".", "root directory to scan (default: current directory)")
	file := flag.String("file", "", "check a single Go file instead of scanning a directory")
	checkerName := flag.String("checker", "", "run only the named checker (e.g. mem-prealloc); use -list-checkers to see all names")
	group := flag.String("group", "", "run only checkers in a theme group: memory | concurrency | io")
	skipVendor := flag.Bool("skip-vendor", true, "skip the vendor/ directory")
	skipTests := flag.Bool("skip-tests", false, "skip *_test.go files")
	skipGenerated := flag.Bool("skip-generated", true, "skip files containing a '// Code generated' header")
	exclude := flag.String("exclude", "", "comma-separated list of exclusion patterns: directory names end with / (e.g. mocks/), file globs do not (e.g. *_gen.go,*.pb.go)")
	severity := flag.String("severity", "INFO", "minimum severity to report: INFO | WARN | ERROR")
	gitStaged := flag.Bool("git-staged", false, "only check Go files staged for the next git commit")
	gitDiff := flag.Bool("git-diff", false, "check only lines added or modified compared to HEAD (covers staged + unstaged changes)")
	output := flag.String("output", "", "write report to a file: .md for Markdown, .sarif for SARIF 2.1.0 (GitHub Code Scanning)")
	format := flag.String("format", "text", "output format: text | json")
	workers := flag.Int("workers", defaultWorkers(), "number of parallel workers for file scanning")
	fix := flag.Bool("fix", false, "auto-apply fixable suggestions in place (modifies source files)")
	noColor := flag.Bool("no-color", false, "disable emoji and Unicode box-drawing in output (also respects NO_COLOR env var)")
	cache := flag.Bool("cache", true, "cache parse+check results in .goperfcheck-cache/ (keyed by file hash); near-instant re-runs on unchanged files; disable with -cache=false or -cache false")
	stdin := flag.Bool("stdin", false, "read Go source from stdin instead of a file or directory")
	listCheckers := flag.Bool("list-checkers", false, "print all checkers with severity, group, and description, then exit")
	auditSuppressions := flag.Bool("audit-suppressions", false, "report stale and expired //goperfcheck:ignore comments, then exit")
	goVersion := flag.String("go-version", "", "Go version the project targets, e.g. 1.22 (default: auto-detect from go.mod)")
	showVersion := flag.Bool("version", false, "print version and exit")

	// Load .goperfcheck config before flag.Parse so CLI flags take precedence.
	cfg, cfgPath, cfgErr := loadConfig()
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", cfgErr)
		os.Exit(2)
	}
	for k, v := range cfg {
		// Paths from the config file must be relative and must not escape the
		// project root via "..". CLI flags are not restricted — the user
		// controls their own shell. This prevents a malicious .goperfcheck file
		// in a checked-out repo from writing reports to arbitrary system paths.
		if (k == "output" || k == "dir") && v != "" {
			if filepath.IsAbs(v) || strings.Contains(filepath.Clean(v), "..") {
				fmt.Fprintf(os.Stderr, "error: %s: %q must be a relative path within the project, got %q\n", cfgPath, k, v)
				os.Exit(2)
			}
		}
		if setErr := flag.Set(k, v); setErr != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: unknown or invalid setting %q = %q\n", cfgPath, k, v)
		}
	}

	// Normalize "-flag false" → "-flag=false" for all boolean flags before
	// flag.Parse sees them. Go's flag package treats boolean flags specially:
	// "-flag value" does not assign value to flag; instead flag is set to true
	// and value becomes a positional argument. Normalizing first lets users
	// write "-cache false" (space-separated) as naturally as "-cache=false".
	var boolFlagNames []string
	flag.VisitAll(func(f *flag.Flag) {
		type boolFlagger interface{ IsBoolFlag() bool }
		if bf, ok := f.Value.(boolFlagger); ok && bf.IsBoolFlag() {
			boolFlagNames = append(boolFlagNames, f.Name)
		}
	})
	if err := flag.CommandLine.Parse(normalizeSpacedBoolFlags(os.Args[1:], boolFlagNames)); err != nil {
		os.Exit(2)
	}

	_, noColorEnv := os.LookupEnv("NO_COLOR")
	plain := *noColor || noColorEnv
	colorEnabled := !plain && isTerminal()

	symOK := "✓"
	symFile := "📁"
	symWarn := "⚠ "
	symHint := "💡"
	symBench := "📊"
	symConf := "🔍"
	symVer := "📦"
	symSep := "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	if plain {
		symOK = "[OK]"
		symFile = "--"
		symWarn = "!"
		symHint = "hint:"
		symBench = "bench:"
		symConf = "fp?:"
		symVer = "ver:"
		symSep = "-------------------------------------------------"
	}

	if *showVersion {
		fmt.Printf("goperfcheck v%s\n", version)
		return
	}

	// "explain" is a subcommand, not a flag: goperfcheck explain <CheckerName>
	// Any other positional argument is an unrecognized subcommand — reject it
	// rather than silently scanning the current directory.
	if args := flag.Args(); len(args) >= 1 {
		if strings.EqualFold(args[0], "explain") {
			runExplain(args[1:], colorEnabled)
			return
		}
		fmt.Fprintf(os.Stderr, "error: unknown subcommand %q\n\nKnown subcommands:\n  explain   print what a checker looks for and code examples\n\nRun 'goperfcheck -help' for flag usage.\n", args[0])
		os.Exit(2)
	}

	if *listCheckers {
		fmt.Printf("%-17s %-9s %-8s %-13s %s\n", "NAME", "SEVERITY", "CONF", "GROUP", "DESCRIPTION")
		fmt.Printf("%s %s %s %s %s\n",
			strings.Repeat("-", 17), strings.Repeat("-", 9),
			strings.Repeat("-", 8), strings.Repeat("-", 13), strings.Repeat("-", 44))
		for _, c := range checker.AllCheckers() {
			meta := checker.Metadata[c.Name()]
			fmt.Printf("%-17s %s %s %-13s %s\n",
				c.Name(),
				coloredSeverityPadded(meta.Severity, 8, colorEnabled),
				coloredConfidencePadded(meta.Confidence, 8, colorEnabled),
				meta.Group,
				meta.Description)
		}
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
	if *stdin && *auditSuppressions {
		fmt.Fprintf(os.Stderr, "error: -audit-suppressions cannot be used with -stdin\n")
		os.Exit(1)
	}

	if *gitDiff && *gitStaged {
		fmt.Fprintf(os.Stderr, "error: -git-diff and -git-staged cannot be used together\n")
		os.Exit(1)
	}
	if *gitDiff && *stdin {
		fmt.Fprintf(os.Stderr, "error: -git-diff and -stdin cannot be used together\n")
		os.Exit(1)
	}
	if *gitDiff && *file != "" {
		fmt.Fprintf(os.Stderr, "error: -git-diff and -file cannot be used together\n")
		os.Exit(1)
	}

	if *checkerName != "" && *group != "" {
		fmt.Fprintf(os.Stderr, "error: -checker and -group cannot be used together\n")
		os.Exit(1)
	}

	minSev := parseSeverity(*severity)

	// --- Go version detection ---
	// Determines which go.mod-based adjustments to apply (ApplyGoVersion).
	var detectedGoVer checker.GoVersion
	var goVerSource string
	if *goVersion != "" {
		if v, ok := checker.ParseGoVersion(*goVersion); ok {
			detectedGoVer = v
			goVerSource = "flag"
		} else {
			fmt.Fprintf(os.Stderr, "warning: invalid -go-version %q — expected X.Y or X.Y.Z; ignoring\n", *goVersion)
		}
	} else if !*stdin {
		searchRoot := *dir
		if *file != "" {
			searchRoot = filepath.Dir(*file)
		}
		if abs, err := filepath.Abs(searchRoot); err == nil {
			if v, ok := checker.ReadModGoVersion(abs); ok {
				detectedGoVer = v
				goVerSource = "go.mod"
			}
		}
	}

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

	excludePatterns := parseExcludePatterns(*exclude)

	var allIssues []checker.Issue
	var gitDiffResult *diffResult // non-nil when -git-diff is active

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
				p := filepath.Join(root, name)
				if !fileExcluded(p, excludePatterns) {
					paths = append(paths, p)
				}
			}
		} else if *gitDiff {
			root, absErr := filepath.Abs(*dir)
			if absErr != nil {
				fmt.Fprintf(os.Stderr, "abs error: %v\n", absErr)
				os.Exit(1)
			}
			dr, diffErr := runGitDiff(root)
			if diffErr != nil {
				fmt.Fprintf(os.Stderr, "git error: %v\n", diffErr)
				os.Exit(1)
			}
			gitDiffResult = dr
			for _, p := range dr.files {
				if !strings.HasSuffix(p, ".go") {
					continue
				}
				if !fileExcluded(p, excludePatterns) {
					paths = append(paths, p)
				}
			}
			if len(paths) == 0 {
				fmt.Printf("%s No Go changes detected in working tree or index\n", symOK)
				return
			}
		} else {
			err := filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					// Never filter the root itself — filepath.Base(".") == "."
					// which would incorrectly SkipDir the entire tree.
					if path != *dir {
						base := filepath.Base(path)
						if *skipVendor && base == "vendor" {
							return filepath.SkipDir
						}
						if strings.HasPrefix(base, ".") {
							return filepath.SkipDir
						}
						if dirExcluded(base, excludePatterns) {
							return filepath.SkipDir
						}
					}
					return nil
				}
				if !strings.HasSuffix(path, ".go") {
					return nil
				}
				if !fileExcluded(path, excludePatterns) {
					paths = append(paths, path)
				}
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
		if numW > 512 {
			numW = 512
		}
		// Skip generated files for directory/git-staged scans but not for
		// explicit -file (user intent takes precedence).
		useSkipGenerated := *skipGenerated && *file == ""

		if *auditSuppressions {
			today := time.Now().Truncate(24 * time.Hour)
			// Always run all checkers for accurate stale detection regardless of
			// -checker/-group filters; the goal is to find dead suppression comments.
			allSuppressed := auditFiles(paths, checker.AllCheckers(), *skipTests, useSkipGenerated, numW, today)
			if printAuditReport(allSuppressed, *dir, plain) {
				os.Exit(1)
			}
			return
		}

		absRoot, _ := filepath.Abs(*dir)
		cc := buildCacheConfig(*cache, filepath.Join(absRoot, cacheDirName), allCheckers)

		var filesDone atomic.Int64
		raw := runScanWithProgress(paths, allCheckers, *skipTests, useSkipGenerated, numW, cc, &filesDone)
		for _, issue := range raw {
			if severityLevel(issue.Severity) >= severityLevel(minSev) {
				allIssues = append(allIssues, issue)
			}
		}
	}

	// For -git-diff mode, restrict issues to lines actually changed in the diff.
	if gitDiffResult != nil {
		allIssues = filterByChangedLines(allIssues, gitDiffResult.changedLines)
	}

	// Deduplicate issues (can occur with nested loops)
	allIssues = checker.DedupeIssues(allIssues)

	// Apply Go version-aware adjustments (modifies suggestions, sets VersionNote).
	allIssues = checker.ApplyGoVersion(allIssues, detectedGoVer)

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
		case *gitDiff:
			fmt.Printf("%s No performance issues found in changed lines\n", symOK)
		case *gitStaged:
			fmt.Printf("%s No performance issues found in staged files\n", symOK)
		default:
			fmt.Printf("%s No performance issues found in %s\n", symOK, *dir)
		}
		printGoVersionLine(detectedGoVer, goVerSource, *goVersion, *stdin, plain)
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
		fmt.Printf("   %s %s  %s\n", coloredSeverity(issue.Severity, colorEnabled), fileLink(issue.File, rel, issue.Line, issue.Column, colorEnabled), issue.Checker)
		fmt.Printf("   %s %s\n", symWarn, issue.Message)
		if issue.Suggestion != "" {
			fmt.Printf("   %s %s\n", symHint, issue.Suggestion)
		}
		if issue.Benchmark != "" {
			fmt.Printf("   %s %s\n", symBench, issue.Benchmark)
		}
		if meta, ok := checker.Metadata[issue.Checker]; ok &&
			meta.Confidence == checker.ConfidenceLow &&
			meta.FalsePositiveNote != "" {
			fmt.Printf("   %s Low confidence: %s\n", symConf, meta.FalsePositiveNote)
		}
		if issue.VersionNote != "" {
			fmt.Printf("   %s %s\n", symVer, issue.VersionNote)
		}
	}

	var nErr, nWarn, nInfo int
	for _, issue := range allIssues {
		switch issue.Severity {
		case checker.SeverityError:
			nErr++
		case checker.SeverityWarning:
			nWarn++
		default:
			nInfo++
		}
	}
	fmt.Printf("\n%s\n", symSep)
	fmt.Printf("Found %d performance issue(s): %s · %s · %s\n",
		len(allIssues),
		coloredCount(nErr, checker.SeverityError, colorEnabled),
		coloredCount(nWarn, checker.SeverityWarning, colorEnabled),
		coloredCount(nInfo, checker.SeverityInfo, colorEnabled))
	printGoVersionLine(detectedGoVer, goVerSource, *goVersion, *stdin, plain)

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

// runExplain prints the full explanation for a named checker and exits.
// args contains the positional arguments after "explain"; colorEnabled controls ANSI output.
func runExplain(args []string, colorEnabled bool) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: goperfcheck explain <checker-name>")
		fmt.Fprintln(os.Stderr, "\nAvailable checkers:")
		for _, c := range checker.AllCheckers() {
			meta := checker.Metadata[c.Name()]
			fmt.Fprintf(os.Stderr, "  %-17s %s\n", c.Name(), meta.Description)
		}
		os.Exit(1)
	}

	name := args[0]
	var canonicalName string
	var meta checker.CheckerMeta
	for n, m := range checker.Metadata {
		if strings.EqualFold(n, name) {
			canonicalName = n
			meta = m
			break
		}
	}
	if canonicalName == "" {
		fmt.Fprintf(os.Stderr, "unknown checker %q\n\nRun 'goperfcheck explain' to list all checkers.\n", name)
		os.Exit(1)
	}

	sep := "─────────────────────────────────────────────────"
	fmt.Printf("Checker:     %s\n", canonicalName)
	fmt.Printf("Severity:    %s\n", coloredSeverity(checker.Severity(meta.Severity), colorEnabled))
	fmt.Printf("Confidence:  %s\n", coloredConfidencePadded(meta.Confidence, 0, colorEnabled))
	fmt.Printf("Group:       %s\n", meta.Group)
	fmt.Printf("Description: %s\n", meta.Description)
	if meta.Link != "" {
		fmt.Printf("Docs:        %s\n", meta.Link)
	}
	if meta.FalsePositiveNote != "" {
		fmt.Printf("\n%s\n", sep)
		fmt.Println("Limitations (AST-only analysis):")
		for _, line := range strings.Split(meta.FalsePositiveNote, "\n") {
			fmt.Printf("  %s\n", line)
		}
	}
	if meta.BadExample != "" {
		fmt.Printf("\n%s\n", sep)
		fmt.Println("Bad (will trigger):")
		for _, line := range strings.Split(meta.BadExample, "\n") {
			fmt.Printf("  %s\n", line)
		}
	}
	if meta.GoodExample != "" {
		fmt.Printf("\n%s\n", sep)
		fmt.Println("Good (preferred):")
		for _, line := range strings.Split(meta.GoodExample, "\n") {
			fmt.Printf("  %s\n", line)
		}
	}
	fmt.Printf("\n%s\n", sep)
}

// normalizeSpacedBoolFlags rewrites "-flag false" and "--flag false" to
// "-flag=false" (and equivalently for "true", "1", "0") for every flag name in
// boolFlagNames, before flag.Parse processes them.
//
// Go's flag package treats boolean flags specially: "-flag value" sets the flag
// to true and leaves "value" as a positional argument, because booleans are
// allowed without an explicit value ("-flag" alone means true). This helper
// bridges the gap so users can write either form interchangeably.
func normalizeSpacedBoolFlags(args []string, boolFlagNames []string) []string {
	known := make(map[string]bool, len(boolFlagNames))
	for _, n := range boolFlagNames {
		known[n] = true
	}

	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if !strings.HasPrefix(tok, "-") {
			out = append(out, tok)
			continue
		}
		// Strip one or two leading dashes to get the bare flag name.
		name := strings.TrimLeft(tok, "-")
		// Already has an inline value (-flag=value): pass through unchanged.
		if strings.ContainsRune(name, '=') {
			out = append(out, tok)
			continue
		}
		// If this is a known bool flag and the very next token is a bare boolean
		// literal, merge them with "=" so the flag package accepts the pair.
		if known[name] && i+1 < len(args) {
			switch strings.ToLower(args[i+1]) {
			case "true", "false", "1", "0":
				out = append(out, tok+"="+args[i+1])
				i++ // consume the value token
				continue
			}
		}
		out = append(out, tok)
	}
	return out
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
