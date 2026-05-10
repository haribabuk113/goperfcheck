package checker

import (
	"go/ast"
	"go/token"
	"sort"
	"strings"
	"time"
)

// suppressEntry holds the parsed state of one //goperfcheck:ignore comment.
type suppressEntry struct {
	all      bool
	checkers map[string]bool
	until    time.Time // zero = no expiry
}

func (e *suppressEntry) expired(today time.Time) bool {
	return !e.until.IsZero() && today.After(e.until)
}

// SuppressedIssue describes one //goperfcheck:ignore comment found during a
// suppression audit. Returned by CollectSuppressions.
type SuppressedIssue struct {
	File     string
	Line     int
	Checkers []string // nil/empty means the comment suppresses all checkers
	Until    string   // "YYYY-MM-DD" expiry annotation, or "" if none
	Stale    bool     // true when no matching checker fires on the covered lines
	Expired  bool     // true when the Until date has passed
}

// parseSuppressionRest parses the text following "goperfcheck:ignore".
// It extracts an optional until:YYYY-MM-DD token and the remaining checker
// names (comma-separated). Returns nil names + isAll=true when no checker
// names are present.
func parseSuppressionRest(rest string) (names []string, isAll bool, until time.Time, untilStr string) {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return nil, true, time.Time{}, ""
	}

	var checkerTokens []string
	for _, tok := range strings.Fields(rest) {
		if after, ok := strings.CutPrefix(tok, "until:"); ok {
			if t, err := time.Parse("2006-01-02", after); err == nil {
				until = t
				untilStr = after
			}
		} else {
			checkerTokens = append(checkerTokens, tok)
		}
	}

	// Checker names may be comma-separated within a single space-separated token.
	for _, tok := range checkerTokens {
		for _, name := range strings.Split(tok, ",") {
			if n := strings.TrimSpace(name); n != "" {
				names = append(names, n)
			}
		}
	}
	if len(names) == 0 {
		isAll = true
	}
	return names, isAll, until, untilStr
}

// buildSuppressions parses all //goperfcheck:ignore comments in file into a
// map keyed by line number. Shared by FilterSuppressed and CollectSuppressions.
func buildSuppressions(fset *token.FileSet, file *ast.File) map[int]*suppressEntry {
	sups := make(map[int]*suppressEntry)
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			text := strings.TrimSpace(strings.TrimLeft(c.Text, "/"))
			if !strings.HasPrefix(text, "goperfcheck:ignore") {
				continue
			}
			rest := strings.TrimSpace(strings.TrimPrefix(text, "goperfcheck:ignore"))
			line := fset.Position(c.Slash).Line
			e := sups[line]
			if e == nil {
				e = &suppressEntry{checkers: make(map[string]bool)}
				sups[line] = e
			}
			names, isAll, until, _ := parseSuppressionRest(rest)
			if isAll {
				e.all = true
			} else {
				for _, n := range names {
					e.checkers[n] = true
				}
			}
			if !until.IsZero() {
				e.until = until
			}
		}
	}
	return sups
}

// FilterSuppressed removes issues covered by active //goperfcheck:ignore comments.
//
// A suppression comment on line N covers issues on line N (same-line) and
// line N+1 (comment placed on the line above the offending code).
//
// Formats accepted (with or without a space after //):
//
//	//goperfcheck:ignore                                suppress all checkers
//	//goperfcheck:ignore MemPrealloc                   suppress one checker
//	//goperfcheck:ignore MemPrealloc,StructAlign        suppress multiple
//	//goperfcheck:ignore MemPrealloc until:2026-08-01  suppress until a date
//	//goperfcheck:ignore until:2026-08-01              suppress all until a date
//
// Suppressions whose until:YYYY-MM-DD date has passed are ignored — the
// issue resurfaces automatically so the team is reminded to re-evaluate.
func FilterSuppressed(fset *token.FileSet, file *ast.File, issues []Issue) []Issue {
	if len(issues) == 0 || len(file.Comments) == 0 {
		return issues
	}

	today := time.Now().Truncate(24 * time.Hour)
	sups := buildSuppressions(fset, file)
	if len(sups) == 0 {
		return issues
	}

	out := issues[:0:0]
	for _, iss := range issues {
		if !suppressed(sups, iss, today) {
			out = append(out, iss)
		}
	}
	return out
}

func suppressed(sups map[int]*suppressEntry, iss Issue, today time.Time) bool {
	for _, line := range [2]int{iss.Line, iss.Line - 1} {
		if e, ok := sups[line]; ok {
			if e.expired(today) {
				continue
			}
			if e.all || e.checkers[iss.Checker] {
				return true
			}
		}
	}
	return false
}

// CollectSuppressions audits all //goperfcheck:ignore comments in file,
// checking each against rawIssues (unfiltered checker output) to determine
// whether the suppression is stale (the covered checker no longer fires) or
// expired (an until: date has passed today).
//
// rawIssues must be the raw output of all checkers run on this file before
// any call to FilterSuppressed.
func CollectSuppressions(fset *token.FileSet, file *ast.File, rawIssues []Issue, today time.Time) []SuppressedIssue {
	if len(file.Comments) == 0 {
		return nil
	}

	filename := fset.File(file.Pos()).Name()

	// Collect suppression comments, preserving the original until string for
	// display purposes (buildSuppressions discards it).
	type rawEntry struct {
		entry    *suppressEntry
		untilStr string
	}
	rawEntries := make(map[int]*rawEntry)

	for _, cg := range file.Comments {
		for _, c := range cg.List {
			text := strings.TrimSpace(strings.TrimLeft(c.Text, "/"))
			if !strings.HasPrefix(text, "goperfcheck:ignore") {
				continue
			}
			rest := strings.TrimSpace(strings.TrimPrefix(text, "goperfcheck:ignore"))
			line := fset.Position(c.Slash).Line

			names, isAll, until, untilStr := parseSuppressionRest(rest)
			re := rawEntries[line]
			if re == nil {
				re = &rawEntry{entry: &suppressEntry{checkers: make(map[string]bool)}}
				rawEntries[line] = re
			}
			if isAll {
				re.entry.all = true
			} else {
				for _, n := range names {
					re.entry.checkers[n] = true
				}
			}
			if !until.IsZero() {
				re.entry.until = until
				re.untilStr = untilStr
			}
		}
	}

	if len(rawEntries) == 0 {
		return nil
	}

	// Index raw issues for quick stale-detection lookups.
	type issKey struct {
		line    int
		checker string
	}
	issLines := make(map[int]bool)    // any issue fires on this line?
	issByKey := make(map[issKey]bool) // specific checker fires on this line?
	for _, iss := range rawIssues {
		issLines[iss.Line] = true
		issByKey[issKey{iss.Line, iss.Checker}] = true
	}

	var result []SuppressedIssue
	for line, re := range rawEntries {
		// A suppression covers lines [line] and [line+1].
		var stale bool
		if re.entry.all {
			stale = !issLines[line] && !issLines[line+1]
		} else {
			fires := false
			for n := range re.entry.checkers {
				if issByKey[issKey{line, n}] || issByKey[issKey{line + 1, n}] {
					fires = true
					break
				}
			}
			stale = !fires
		}

		var checkers []string
		for n := range re.entry.checkers {
			checkers = append(checkers, n)
		}
		sort.Strings(checkers)

		result = append(result, SuppressedIssue{
			File:     filename,
			Line:     line,
			Checkers: checkers, // nil when re.entry.all is true
			Until:    re.untilStr,
			Stale:    stale,
			Expired:  re.entry.expired(today),
		})
	}
	return result
}
