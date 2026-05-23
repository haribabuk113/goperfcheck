package main

import (
	"path/filepath"
	"testing"

	"github.com/haribabuk113/goperfcheck/checker"
)

func TestParseNewStart(t *testing.T) {
	tests := []struct {
		header string
		want   int
	}{
		{"@@ -1,4 +1,6 @@ func foo() {", 1},
		{"@@ -10,7 +12,8 @@", 12},
		{"@@ -0,0 +1 @@", 1},
		{"@@ -5,0 +6,3 @@", 6},
		{"@@ -1 +1 @@", 1},
		{"@@ -20,4 +25,4 @@ func bar() {", 25},
		{"malformed", 1},
		{"", 1},
	}
	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			got := parseNewStart(tt.header)
			if got != tt.want {
				t.Errorf("parseNewStart(%q) = %d, want %d", tt.header, got, tt.want)
			}
		})
	}
}

func TestParseDiffSingleFile(t *testing.T) {
	const root = "/repo"
	// Simulates a diff where lines 3 and 4 are added to foo.go.
	// Line layout in new file:
	//   1: package main       (context)
	//   2: (blank)            (context)
	//   3: // added comment   (+)
	//   4: func newFunc() {}  (+)
	//   5: func existing() {} (context)
	diff := []byte(`diff --git a/foo.go b/foo.go
index abc..def 100644
--- a/foo.go
+++ b/foo.go
@@ -1,4 +1,6 @@
 package main

+// added comment
+func newFunc() {}
 func existing() {}
-// old line
`)

	dr := parseDiff(root, diff)

	abs := filepath.Join(root, "foo.go")
	if len(dr.files) != 1 || dr.files[0] != abs {
		t.Fatalf("files = %v, want [%s]", dr.files, abs)
	}

	lines := dr.changedLines[abs]
	for _, wantChanged := range []int{3, 4} {
		if !lines[wantChanged] {
			t.Errorf("expected line %d to be marked changed", wantChanged)
		}
	}
	for _, wantUnchanged := range []int{1, 2, 5} {
		if lines[wantUnchanged] {
			t.Errorf("line %d is context/deleted — should not be marked changed", wantUnchanged)
		}
	}
}

func TestParseDiffMultipleFiles(t *testing.T) {
	const root = "/repo"
	// a.go: line 2 added; b.go: line 6 added.
	diff := []byte(`diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,2 +1,3 @@
 package a
+var x = 1
 func A() {}
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -5,3 +5,4 @@
 func B() {}
+func BNew() {}
 // end
`)

	dr := parseDiff(root, diff)

	if len(dr.files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(dr.files), dr.files)
	}

	aLines := dr.changedLines[filepath.Join(root, "a.go")]
	if !aLines[2] {
		t.Errorf("a.go line 2 should be changed")
	}
	if aLines[1] || aLines[3] {
		t.Errorf("a.go lines 1 and 3 are context, should not be marked changed")
	}

	bLines := dr.changedLines[filepath.Join(root, "b.go")]
	if !bLines[6] {
		t.Errorf("b.go line 6 should be changed")
	}
	if bLines[5] || bLines[7] {
		t.Errorf("b.go lines 5 and 7 are context, should not be marked changed")
	}
}

func TestParseDiffNewFile(t *testing.T) {
	const root = "/repo"
	// Entirely new file — all lines are additions.
	diff := []byte(`diff --git a/new.go b/new.go
new file mode 100644
index 000000..abc1234
--- /dev/null
+++ b/new.go
@@ -0,0 +1,3 @@
+package main
+
+func Hello() {}
`)

	dr := parseDiff(root, diff)
	abs := filepath.Join(root, "new.go")

	if len(dr.files) != 1 || dr.files[0] != abs {
		t.Fatalf("files = %v, want [%s]", dr.files, abs)
	}
	lines := dr.changedLines[abs]
	for _, wantChanged := range []int{1, 2, 3} {
		if !lines[wantChanged] {
			t.Errorf("new file: expected line %d to be marked changed", wantChanged)
		}
	}
}

func TestParseDiffMultipleHunks(t *testing.T) {
	const root = "/repo"
	// Two non-contiguous hunks in the same file.
	// Hunk 1 (starts at new-file line 1): lines 1,2 context, line 3 added.
	// Hunk 2 (starts at new-file line 19): line 19 context, line 20 added.
	diff := []byte(`diff --git a/foo.go b/foo.go
--- a/foo.go
+++ b/foo.go
@@ -1,4 +1,5 @@
 line1
 line2
+added at 3
 line3
 line4
@@ -18,3 +19,4 @@
 line18
+added at 20
 line19
 line20
`)

	dr := parseDiff(root, diff)
	lines := dr.changedLines[filepath.Join(root, "foo.go")]

	if !lines[3] {
		t.Errorf("hunk 1: expected line 3 to be changed")
	}
	if !lines[20] {
		t.Errorf("hunk 2: expected line 20 to be changed")
	}
	if lines[1] || lines[2] || lines[4] {
		t.Errorf("hunk 1: context lines should not be marked changed")
	}
}

func TestParseDiffEmptyDiff(t *testing.T) {
	dr := parseDiff("/repo", []byte(""))
	if len(dr.files) != 0 {
		t.Errorf("empty diff should produce no files, got %v", dr.files)
	}
}

func TestFilterByChangedLines(t *testing.T) {
	issues := []checker.Issue{
		{File: "/repo/a.go", Line: 5, Checker: "MemPrealloc"},
		{File: "/repo/a.go", Line: 10, Checker: "MemPrealloc"}, // not in changed lines
		{File: "/repo/b.go", Line: 3, Checker: "StructAlign"},
		{File: "/repo/c.go", Line: 1, Checker: "GoroutinePool"}, // file not in diff
	}

	changedLines := map[string]map[int]bool{
		"/repo/a.go": {5: true, 7: true},
		"/repo/b.go": {3: true},
	}

	filtered := filterByChangedLines(issues, changedLines)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered issues, got %d: %+v", len(filtered), filtered)
	}
	if filtered[0].File != "/repo/a.go" || filtered[0].Line != 5 {
		t.Errorf("expected a.go:5, got %s:%d", filtered[0].File, filtered[0].Line)
	}
	if filtered[1].File != "/repo/b.go" || filtered[1].Line != 3 {
		t.Errorf("expected b.go:3, got %s:%d", filtered[1].File, filtered[1].Line)
	}
}

func TestFilterByChangedLinesNoMatch(t *testing.T) {
	issues := []checker.Issue{
		{File: "/repo/a.go", Line: 15, Checker: "MemPrealloc"},
	}
	changedLines := map[string]map[int]bool{
		"/repo/a.go": {5: true},
	}
	filtered := filterByChangedLines(issues, changedLines)
	if len(filtered) != 0 {
		t.Errorf("expected 0 issues when line not in diff, got %d", len(filtered))
	}
}

func TestFilterByChangedLinesEmpty(t *testing.T) {
	issues := []checker.Issue{
		{File: "/repo/a.go", Line: 5, Checker: "MemPrealloc"},
	}
	filtered := filterByChangedLines(issues, map[string]map[int]bool{})
	if len(filtered) != 0 {
		t.Errorf("expected 0 issues for empty changedLines, got %d", len(filtered))
	}
}
