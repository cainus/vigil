package main

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func init() {
	// Tests run without a TTY, where lipgloss would otherwise strip all
	// styling (including Reverse) and make highlighted/plain renders
	// indistinguishable.
	lipgloss.SetColorProfile(termenv.ANSI)
}

func TestNormalizeSelectionOrdersBackwardsDrag(t *testing.T) {
	sel := normalizeSelection(3, 5, 1, 2)
	want := selection{startRow: 1, startCol: 2, endRow: 3, endCol: 5}
	if sel != want {
		t.Fatalf("normalizeSelection = %+v, want %+v", sel, want)
	}
}

func TestSelectionIsEmpty(t *testing.T) {
	if !normalizeSelection(2, 4, 2, 4).isEmpty() {
		t.Fatal("selection with identical start/end should be empty")
	}
	if normalizeSelection(2, 4, 2, 5).isEmpty() {
		t.Fatal("selection spanning one column should not be empty")
	}
}

func TestSelectedTextSingleLine(t *testing.T) {
	rows := []bodyRow{
		newRow(plainSeg("  "), styledSeg("modified", statusModified), plainSeg("  "), styledSeg("main.go", fileStyle)),
	}
	sel := normalizeSelection(0, 12, 0, 19)
	got := selectedText(rows, sel)
	if got != "main.go" {
		t.Fatalf("selectedText = %q, want %q", got, "main.go")
	}
}

func TestSelectedTextMultiLineIncludesWholeMiddleRows(t *testing.T) {
	rows := []bodyRow{
		newRow(plainSeg("Branch Files:")),
		newRow(plainSeg("  "), styledSeg("added", statusAdded), plainSeg("  "), styledSeg("a.go", fileStyle)),
		newRow(plainSeg("  "), styledSeg("deleted", statusDeleted), plainSeg("  "), styledSeg("b.go", fileStyle)),
	}
	sel := normalizeSelection(0, 7, 2, 4)
	got := selectedText(rows, sel)
	want := "Files:\n  added  a.go\n  de"
	if got != want {
		t.Fatalf("selectedText = %q, want %q", got, want)
	}
}

func TestRenderRowsHighlightDoesNotAlterPlainText(t *testing.T) {
	rows := []bodyRow{
		newRow(plainSeg("  "), styledSeg("modified", statusModified), plainSeg("  "), styledSeg("main.go", fileStyle)),
	}
	sel := normalizeSelection(0, 2, 0, 10)
	highlighted := renderRows(rows, &sel)
	plain := renderRows(rows, nil)
	if highlighted == plain {
		t.Fatal("expected highlighted render to differ from plain render")
	}
	if got := rows[0].plainText(); got != "  modified  main.go" {
		t.Fatalf("plainText = %q, want unchanged source text", got)
	}
}
