package main

import (
	"reflect"
	"testing"
)

func TestParseNumstatBasic(t *testing.T) {
	got := parseNumstat([]byte("12\t3\tmain.go\n0\t5\told.go\n"))
	want := map[string]lineStats{
		"main.go": {added: 12, deleted: 3},
		"old.go":  {added: 0, deleted: 5},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseNumstat = %#v, want %#v", got, want)
	}
}

func TestParseNumstatSkipsBinary(t *testing.T) {
	got := parseNumstat([]byte("-\t-\timage.png\n"))
	if len(got) != 0 {
		t.Fatalf("expected binary files to be skipped, got %#v", got)
	}
}

func TestNumstatPathResolvesPlainRename(t *testing.T) {
	if got := numstatPath("old/name.go => new/name.go"); got != "new/name.go" {
		t.Fatalf("numstatPath = %q, want %q", got, "new/name.go")
	}
}

func TestNumstatPathResolvesBraceRename(t *testing.T) {
	if got := numstatPath("backend/{old => new}/file.go"); got != "backend/new/file.go" {
		t.Fatalf("numstatPath = %q, want %q", got, "backend/new/file.go")
	}
}

func TestNumstatPathLeavesPlainPathAlone(t *testing.T) {
	if got := numstatPath("backend/app/main.go"); got != "backend/app/main.go" {
		t.Fatalf("numstatPath = %q, want unchanged", got)
	}
}

func TestStatsSuffixOmittedWhenNoChange(t *testing.T) {
	if segs := statsSuffix(fileEntry{}); segs != nil {
		t.Fatalf("expected no suffix for a zero-change entry, got %#v", segs)
	}
}

func TestFlatFileRowsAppendsStatsOnLastLine(t *testing.T) {
	e := fileEntry{label: symbolModified, style: statusModified, path: "main.go", added: 12, deleted: 3}
	rows := flatFileRows(e, 200)
	if len(rows) != 1 {
		t.Fatalf("expected a single row, got %d", len(rows))
	}
	if got := rows[0].plainText(); got != "  ~ main.go +12 -3" {
		t.Fatalf("got %q", got)
	}
}
