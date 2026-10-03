package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGetWorktreesListsCurrentRepo(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	runGit(t, "init", "-q", "-b", "main")
	runGit(t, "commit", "--allow-empty", "-q", "-m", "init")

	wts := GetWorktrees()
	if len(wts) != 1 {
		t.Fatalf("GetWorktrees() = %d entries, want 1: %#v", len(wts), wts)
	}
	if wts[0].Branch != "main" {
		t.Fatalf("Branch = %q, want %q", wts[0].Branch, "main")
	}
	if wts[0].Detached {
		t.Fatal("expected a non-detached worktree")
	}
}

func TestGetWorktreesListsLinkedWorktree(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	chdir(t, dir)
	runGit(t, "init", "-q", "-b", "main")
	runGit(t, "commit", "--allow-empty", "-q", "-m", "init")

	linkedPath := filepath.Join(filepath.Dir(dir), filepath.Base(dir)+"-linked")
	runGit(t, "worktree", "add", "-q", "-b", "feature", linkedPath)
	t.Cleanup(func() {
		runGit(t, "worktree", "remove", "-f", linkedPath)
	})

	wts := GetWorktrees()
	if len(wts) != 2 {
		t.Fatalf("GetWorktrees() = %d entries, want 2: %#v", len(wts), wts)
	}

	var sawMain, sawFeature bool
	for _, wt := range wts {
		switch wt.Branch {
		case "main":
			sawMain = true
		case "feature":
			sawFeature = true
			if wt.Path != linkedPath {
				t.Fatalf("linked worktree path = %q, want %q", wt.Path, linkedPath)
			}
		}
	}
	if !sawMain || !sawFeature {
		t.Fatalf("expected both main and feature worktrees, got %#v", wts)
	}
}

func runGit(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
