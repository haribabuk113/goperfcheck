package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/haribabuk113/goperfcheck/checker"
)

// Minimal SARIF 2.1.0 types sufficient for GitHub Code Scanning.

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string       `json:"id"`
	ShortDescription sarifMessage `json:"shortDescription"`
	HelpURI          string       `json:"helpUri,omitempty"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region"`
}

type sarifArtifactLocation struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId,omitempty"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn"`
}

// ruleURI extracts a URL from Issue.Rule strings like "Description — https://...".
// Falls back to the goperf.dev home page if no URL is found.
func ruleURI(rule string) string {
	if i := strings.Index(rule, "https://"); i >= 0 {
		return rule[i:]
	}
	return "https://goperf.dev"
}

func severityToSARIFLevel(s checker.Severity) string {
	switch s {
	case checker.SeverityError:
		return "error"
	case checker.SeverityWarning:
		return "warning"
	default:
		return "note"
	}
}

func writeSARIFReport(path string, issues []checker.Issue, dir string) (retErr error) {
	absDir, _ := filepath.Abs(dir)

	// Build one rule entry per unique checker, preserving first-seen order.
	ruleIndex := make(map[string]bool)
	rules := make([]sarifRule, 0)
	for _, iss := range issues {
		if ruleIndex[iss.Checker] {
			continue
		}
		ruleIndex[iss.Checker] = true
		desc := checkerCategory[iss.Checker]
		if desc == "" {
			desc = iss.Checker
		}
		r := sarifRule{
			ID:               iss.Checker,
			ShortDescription: sarifMessage{Text: desc},
			HelpURI:          ruleURI(iss.Rule),
		}
		rules = append(rules, r)
	}

	results := make([]sarifResult, 0, len(issues))
	for _, iss := range issues {
		// Prefer a repo-relative URI so GitHub shows the correct file path.
		uri := filepath.ToSlash(iss.File)
		if rel, err := filepath.Rel(absDir, iss.File); err == nil && !strings.HasPrefix(rel, "..") {
			uri = filepath.ToSlash(rel)
		}

		text := iss.Message
		if iss.Suggestion != "" {
			text += "\n\nSuggestion: " + iss.Suggestion
		}

		results = append(results, sarifResult{
			RuleID: iss.Checker,
			Level:  severityToSARIFLevel(iss.Severity),
			Message: sarifMessage{Text: text},
			Locations: []sarifLocation{
				{
					PhysicalLocation: sarifPhysicalLocation{
						ArtifactLocation: sarifArtifactLocation{
							URI:       uri,
							URIBaseID: "%SRCROOT%",
						},
						Region: sarifRegion{
							StartLine:   iss.Line,
							StartColumn: iss.Column,
						},
					},
				},
			},
		})
	}

	log := sarifLog{
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{
			{
				Tool: sarifTool{
					Driver: sarifDriver{
						Name:           "goperfcheck",
						Version:        version,
						InformationURI: "https://goperf.dev",
						Rules:          rules,
					},
				},
				Results: results,
			},
		},
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && retErr == nil {
			retErr = cerr
		}
	}()

	bw := bufio.NewWriter(f)
	enc := json.NewEncoder(bw)
	enc.SetIndent("", "  ")
	if err := enc.Encode(log); err != nil {
		return err
	}
	return bw.Flush()
}
