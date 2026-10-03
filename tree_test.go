package main

import "testing"

func TestStatusSymbolsAreSingleRune(t *testing.T) {
	for _, sym := range []string{symbolAdded, symbolDeleted, symbolModified, symbolRenamed, symbolCopied, symbolUntracked} {
		if n := len([]rune(sym)); n != 1 {
			t.Fatalf("symbol %q is %d runes, want 1 (breaks column alignment)", sym, n)
		}
	}
}

func TestWrapPathBreaksAfterSlash(t *testing.T) {
	got := wrapPath("backend/automation/src/automation_instrumentation.py", 24)
	want := []string{"backend/automation/src/", "automation_instrumentati", "on.py"}
	if len(got) != len(want) {
		t.Fatalf("wrapPath lines = %d, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWrapPathNoWrapWhenItFits(t *testing.T) {
	got := wrapPath("main.go", 80)
	if len(got) != 1 || got[0] != "main.go" {
		t.Fatalf("wrapPath = %#v, want unwrapped single line", got)
	}
}

func TestWrapPathUnlimitedWhenWidthIsZero(t *testing.T) {
	got := wrapPath("a/very/long/path/that/would/otherwise/wrap/heavily.go", availableWidth(0, 16))
	if len(got) != 1 {
		t.Fatalf("wrapPath with unknown width = %d lines, want 1", len(got))
	}
}

func TestBuildFileTreeRowsGroupsSharedDirectory(t *testing.T) {
	entries := []fileEntry{
		{label: symbolModified, style: statusModified, path: "backend/automation/src/auto/a.go"},
		{label: symbolAdded, style: statusAdded, path: "backend/automation/src/auto/b.go"},
		{label: symbolModified, style: statusModified, path: "README.md"},
	}
	rows := buildFileTreeRows(entries, 200)

	var plain []string
	for _, r := range rows {
		plain = append(plain, r.plainText())
	}

	if plain[0] != "  backend/automation/src/auto/" {
		t.Fatalf("expected directory header first, got %q", plain[0])
	}
	foundA, foundB, foundReadme := false, false, false
	for _, line := range plain {
		if line == "    ~ a.go" {
			foundA = true
		}
		if line == "    + b.go" {
			foundB = true
		}
		if line == "  ~ README.md" {
			foundReadme = true
		}
	}
	if !foundA || !foundB {
		t.Fatalf("grouped files not rendered as basenames under header: %q", plain)
	}
	if !foundReadme {
		t.Fatalf("single root file should render flat with its full path: %q", plain)
	}
}

func TestBuildFileTreeRowsSingleFileInDirStaysFlat(t *testing.T) {
	entries := []fileEntry{
		{label: symbolModified, style: statusModified, path: "docs/CHANGE_CONTROL_POLICY.md"},
	}
	rows := buildFileTreeRows(entries, 200)
	if len(rows) != 1 {
		t.Fatalf("expected a single flat row for a lone file, got %d rows", len(rows))
	}
	if got := rows[0].plainText(); got != "  ~ docs/CHANGE_CONTROL_POLICY.md" {
		t.Fatalf("got %q", got)
	}
}
