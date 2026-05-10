package main

import (
	"fmt"
	"os"

	"github.com/haribabuk113/goperfcheck/checker"
)

const (
	ansiReset  = "\033[0m"
	ansiRed    = "\033[1;31m" // bold red   — ERROR / LOW confidence
	ansiYellow = "\033[1;33m" // bold yellow — WARN / MEDIUM confidence
	ansiGreen  = "\033[1;32m" // bold green  — HIGH confidence
	ansiDim    = "\033[2m"    // dim         — INFO
)

// isTerminal reports whether stdout is connected to an interactive terminal.
// Uses only the standard library — no external dependencies.
func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// isStderrTerminal reports whether stderr is connected to an interactive terminal.
func isStderrTerminal() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// coloredSeverity returns "[SEVERITY]" wrapped with ANSI codes when color is enabled.
func coloredSeverity(s checker.Severity, color bool) string {
	if !color {
		return "[" + string(s) + "]"
	}
	switch s {
	case checker.SeverityError:
		return ansiRed + "[ERROR]" + ansiReset
	case checker.SeverityWarning:
		return ansiYellow + "[WARN]" + ansiReset
	default:
		return ansiDim + "[INFO]" + ansiReset
	}
}

// coloredSeverityPadded returns a severity string with enough trailing spaces
// to fill totalWidth visible columns. ANSI escape codes don't count toward
// width, so padding is computed from the bare severity string length.
func coloredSeverityPadded(sev string, totalWidth int, color bool) string {
	padding := totalWidth - len(sev)
	if padding < 0 {
		padding = 0
	}
	spaces := fmt.Sprintf("%-*s", padding, "")
	if !color {
		return sev + spaces
	}
	var code string
	switch checker.Severity(sev) {
	case checker.SeverityError:
		code = ansiRed
	case checker.SeverityWarning:
		code = ansiYellow
	default:
		code = ansiDim
	}
	return code + sev + ansiReset + spaces
}

// coloredConfidencePadded returns a padded confidence string with ANSI color
// when color is enabled. totalWidth is the visible column width.
func coloredConfidencePadded(conf checker.Confidence, totalWidth int, color bool) string {
	s := string(conf)
	padding := totalWidth - len(s)
	if padding < 0 {
		padding = 0
	}
	spaces := fmt.Sprintf("%-*s", padding, "")
	if !color {
		return s + spaces
	}
	var code string
	switch conf {
	case checker.ConfidenceHigh:
		code = ansiGreen
	case checker.ConfidenceMedium:
		code = ansiYellow
	default: // LOW
		code = ansiRed
	}
	return code + s + ansiReset + spaces
}

// coloredCount formats "N SEVERITY" and applies color when color is enabled
// and n > 0. Zero counts are always printed without color.
func coloredCount(n int, s checker.Severity, color bool) string {
	label := fmt.Sprintf("%d %s", n, string(s))
	if !color || n == 0 {
		return label
	}
	switch s {
	case checker.SeverityError:
		return ansiRed + label + ansiReset
	case checker.SeverityWarning:
		return ansiYellow + label + ansiReset
	default:
		return ansiDim + label + ansiReset
	}
}
