package checker

import (
	"go/ast"
	"go/token"
	"strings"
)

type suppressEntry struct {
	all      bool
	checkers map[string]bool
}

// FilterSuppressed removes issues covered by //goperfcheck:ignore comments.
//
// A suppression comment on line N covers issues on line N (same-line) and
// line N+1 (comment placed on the line above the offending code).
//
// Formats accepted (with or without a space after //):
//
//	//goperfcheck:ignore                    suppress all checkers on the line
//	//goperfcheck:ignore MemPrealloc        suppress one checker
//	//goperfcheck:ignore MemPrealloc,StructAlign  suppress multiple checkers
func FilterSuppressed(fset *token.FileSet, file *ast.File, issues []Issue) []Issue {
	if len(issues) == 0 || len(file.Comments) == 0 {
		return issues
	}

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
			if rest == "" {
				e.all = true
			} else {
				for _, name := range strings.Split(rest, ",") {
					e.checkers[strings.TrimSpace(name)] = true
				}
			}
		}
	}

	if len(sups) == 0 {
		return issues
	}

	out := issues[:0:0]
	for _, iss := range issues {
		if !suppressed(sups, iss) {
			out = append(out, iss)
		}
	}
	return out
}

func suppressed(sups map[int]*suppressEntry, iss Issue) bool {
	// Check same line first, then the line above (preceding-comment style).
	for _, line := range [2]int{iss.Line, iss.Line - 1} {
		if e, ok := sups[line]; ok {
			if e.all || e.checkers[iss.Checker] {
				return true
			}
		}
	}
	return false
}
