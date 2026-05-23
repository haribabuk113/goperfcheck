package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/haribabuk113/goperfcheck/checker"
)

// diffResult holds the files touched by a git diff and the specific new-file
// line numbers that were added (+) in each file.
type diffResult struct {
	files        []string                // absolute paths, in diff order
	changedLines map[string]map[int]bool // abs path → set of added line numbers (1-based)
}

// runGitDiff executes `git diff HEAD` from root, which covers both staged and
// unstaged working-tree changes, and returns the parsed result.
func runGitDiff(root string) (*diffResult, error) {
	cmd := exec.Command("git", "diff", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff HEAD: %w", err)
	}
	return parseDiff(root, out), nil
}

// parseDiff parses unified diff output (from git diff HEAD) and returns which
// lines were added (+) in each file. Deleted and context lines are excluded.
func parseDiff(root string, diffOutput []byte) *diffResult {
	dr := &diffResult{
		changedLines: make(map[string]map[int]bool),
	}
	seen := make(map[string]bool)

	var currentFile string
	newLine := 0
	inHunk := false

	for _, raw := range bytes.Split(diffOutput, []byte("\n")) {
		line := string(raw)
		switch {
		case strings.HasPrefix(line, "+++ b/"):
			rel := strings.TrimPrefix(line, "+++ b/")
			abs := filepath.Join(root, filepath.FromSlash(rel))
			currentFile = abs
			inHunk = false
			if !seen[currentFile] {
				seen[currentFile] = true
				dr.files = append(dr.files, currentFile)
				dr.changedLines[currentFile] = make(map[int]bool)
			}
		case strings.HasPrefix(line, "@@"):
			newLine = parseNewStart(line)
			inHunk = true
		case !inHunk || currentFile == "":
			// diff header, index lines, binary notices, etc.
		case strings.HasPrefix(line, "+++"):
			// skip — already handled above
		case strings.HasPrefix(line, "+"):
			// added line: record it and advance new-file counter
			dr.changedLines[currentFile][newLine] = true
			newLine++
		case strings.HasPrefix(line, "-"):
			// deleted line: does not exist in the new file; don't advance
		case strings.HasPrefix(line, "\\"):
			// "\ No newline at end of file": ignore, don't advance
		default:
			// context line: exists in new file; advance counter
			newLine++
		}
	}
	return dr
}

// parseNewStart extracts the new-file start line number from a unified diff
// hunk header of the form "@@ -old[,n] +new[,n] @@ ...".
func parseNewStart(hunkHeader string) int {
	idx := strings.Index(hunkHeader, " +")
	if idx < 0 {
		return 1
	}
	rest := hunkHeader[idx+2:]
	end := strings.IndexAny(rest, ", @\t")
	if end < 0 {
		end = len(rest)
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// filterByChangedLines returns only the issues whose line falls within the set
// of added/modified lines recorded in changedLines.
func filterByChangedLines(issues []checker.Issue, changedLines map[string]map[int]bool) []checker.Issue {
	var out []checker.Issue
	for _, issue := range issues {
		if lines, ok := changedLines[issue.File]; ok && lines[issue.Line] {
			out = append(out, issue)
		}
	}
	return out
}
