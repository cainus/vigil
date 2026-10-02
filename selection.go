package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// rowSegment is one differently-styled piece of a body row (e.g. a status
// label or a filename), used so a mouse selection can highlight part of a
// row without losing the original coloring of the rest of it.
type rowSegment struct {
	text  string
	style lipgloss.Style
}

// bodyRow is one line of the rendered body, broken into segments.
type bodyRow struct {
	segments []rowSegment
}

func plainSeg(text string) rowSegment {
	return rowSegment{text: text, style: lipgloss.NewStyle()}
}

func styledSeg(text string, style lipgloss.Style) rowSegment {
	return rowSegment{text: text, style: style}
}

func newRow(segments ...rowSegment) bodyRow {
	return bodyRow{segments: segments}
}

// plainText returns the row's text with no styling applied.
func (r bodyRow) plainText() string {
	var b strings.Builder
	for _, seg := range r.segments {
		b.WriteString(seg.text)
	}
	return b.String()
}

// selection is a normalized (start <= end) mouse selection in row/column
// space, where columns are rune offsets into a row's plain text.
type selection struct {
	startRow, startCol int
	endRow, endCol     int
}

func normalizeSelection(sr, sc, er, ec int) selection {
	if sr > er || (sr == er && sc > ec) {
		sr, sc, er, ec = er, ec, sr, sc
	}
	return selection{startRow: sr, startCol: sc, endRow: er, endCol: ec}
}

// isEmpty reports whether the selection spans zero characters (e.g. a click
// with no drag).
func (s selection) isEmpty() bool {
	return s.startRow == s.endRow && s.startCol == s.endCol
}

// rowBounds returns the highlighted rune range [start, end) for rowIdx given
// the row's plain-text rune length, and whether the row is touched at all.
func (s selection) rowBounds(rowIdx, lineLen int) (start, end int, ok bool) {
	if rowIdx < s.startRow || rowIdx > s.endRow {
		return 0, 0, false
	}
	start, end = 0, lineLen
	if rowIdx == s.startRow {
		start = s.startCol
	}
	if rowIdx == s.endRow {
		end = s.endCol
	}
	start = clampInt(start, 0, lineLen)
	end = clampInt(end, 0, lineLen)
	if start > end {
		start = end
	}
	return start, end, true
}

func clampInt(v, lo, hi int) int {
	return max(lo, min(v, hi))
}

// renderRows renders every row, applying a reverse-video highlight over the
// selection (if any) while preserving each segment's original style outside
// the highlighted range.
func renderRows(rows []bodyRow, sel *selection) string {
	lines := make([]string, len(rows))
	for i, row := range rows {
		var hStart, hEnd int
		var has bool
		if sel != nil {
			plainLen := len([]rune(row.plainText()))
			hStart, hEnd, has = sel.rowBounds(i, plainLen)
			if has && hStart == hEnd {
				has = false
			}
		}
		lines[i] = renderRow(row, hStart, hEnd, has)
	}
	return strings.Join(lines, "\n")
}

func renderRow(row bodyRow, hStart, hEnd int, highlight bool) string {
	if !highlight {
		var b strings.Builder
		for _, seg := range row.segments {
			b.WriteString(seg.style.Render(seg.text))
		}
		return b.String()
	}

	var b strings.Builder
	pos := 0
	for _, seg := range row.segments {
		runes := []rune(seg.text)
		segStart, segEnd := pos, pos+len(runes)
		pos = segEnd

		a, c := max(hStart, segStart), min(hEnd, segEnd)
		if a >= c {
			b.WriteString(seg.style.Render(seg.text))
			continue
		}

		localA, localC := a-segStart, c-segStart
		if before := string(runes[:localA]); before != "" {
			b.WriteString(seg.style.Render(before))
		}
		b.WriteString(seg.style.Reverse(true).Render(string(runes[localA:localC])))
		if after := string(runes[localC:]); after != "" {
			b.WriteString(seg.style.Render(after))
		}
	}
	return b.String()
}

// selectedText extracts the plain text covered by sel from rows, joining
// multi-row selections with newlines, the way terminal selection does.
func selectedText(rows []bodyRow, sel selection) string {
	if sel.isEmpty() {
		return ""
	}
	var lines []string
	for r := sel.startRow; r <= sel.endRow && r < len(rows); r++ {
		plain := []rune(rows[r].plainText())
		start, end, ok := sel.rowBounds(r, len(plain))
		if !ok {
			continue
		}
		lines = append(lines, string(plain[start:end]))
	}
	return strings.Join(lines, "\n")
}
