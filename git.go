package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// FileChange represents a changed file in git status
type FileChange struct {
	Staged   byte // first column: staged status
	Unstaged byte // second column: unstaged status
	Label    string
	File     string
	Added    int
	Deleted  int
}

// IsGitRepo checks if the current directory is inside a git repository
func IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	err := cmd.Run()
	return err == nil
}

// GetRepoName returns the name of the git repository (basename of the top-level directory)
func GetRepoName() string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return filepath.Base(strings.TrimSpace(string(output)))
}

// ListFiles returns the files and directories in the given directory
func ListFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		files = append(files, name)
	}
	return files
}

// GetCurrentBranch returns the current git branch name
func GetCurrentBranch() string {
	cmd := exec.Command("git", "branch", "--show-current")
	output, err := cmd.Output()
	if err == nil {
		branch := strings.TrimSpace(string(output))
		if branch != "" {
			return branch
		}
	}

	// Try symbolic-ref for repos with no commits yet
	cmd = exec.Command("git", "symbolic-ref", "--short", "HEAD")
	output, err = cmd.Output()
	if err == nil {
		branch := strings.TrimSpace(string(output))
		if branch != "" {
			return branch + " (no commits)"
		}
	}

	// Might be in detached HEAD state
	cmd = exec.Command("git", "rev-parse", "--short", "HEAD")
	output, err = cmd.Output()
	if err == nil {
		return "(detached) " + strings.TrimSpace(string(output))
	}

	return "unknown"
}

func GetHeadRevision() string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// GetGitStatus returns a list of changed files from git status
func GetGitStatus() []FileChange {
	changes, _ := GetGitStatusWithError()
	return changes
}

// GetGitStatusWithError returns changed files and preserves git status failures
// so callers can avoid presenting a failed read as a clean tree.
func GetGitStatusWithError() ([]FileChange, error) {
	cmd := exec.Command("git", "status", "--porcelain", "-uall")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var changes []FileChange
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		staged := line[0]
		unstaged := line[1]
		file := line[3:]
		// Renamed/copied entries are "old -> new"; show the current (new) path.
		if idx := strings.Index(file, " -> "); idx != -1 {
			file = file[idx+len(" -> "):]
		}

		label := statusLabel(staged, unstaged)
		changes = append(changes, FileChange{
			Staged:   staged,
			Unstaged: unstaged,
			Label:    label,
			File:     file,
		})
	}

	stats := getWorkingTreeLineStats()
	for i := range changes {
		if s, ok := stats[changes[i].File]; ok {
			changes[i].Added, changes[i].Deleted = s.added, s.deleted
		}
	}
	return changes, nil
}

type lineStats struct{ added, deleted int }

// getWorkingTreeLineStats returns added/deleted line counts for every file
// with staged or unstaged changes, keyed by its current path. Untracked
// files have no diff to count and are left out.
func getWorkingTreeLineStats() map[string]lineStats {
	stats := parseNumstat(runNumstat("git", "diff", "--numstat"))
	for path, s := range parseNumstat(runNumstat("git", "diff", "--cached", "--numstat")) {
		existing := stats[path]
		existing.added += s.added
		existing.deleted += s.deleted
		stats[path] = existing
	}
	return stats
}

func runNumstat(name string, args ...string) []byte {
	output, err := exec.Command(name, args...).Output()
	if err != nil {
		return nil
	}
	return output
}

// parseNumstat parses `git diff --numstat` output into per-path line stats,
// resolving renamed paths (plain "old => new" and the common-prefix
// "dir/{old => new}/suffix" form) to the current path. Binary files report
// "-" for both counts and are skipped.
func parseNumstat(output []byte) map[string]lineStats {
	stats := map[string]lineStats{}
	if len(output) == 0 {
		return stats
	}
	for _, line := range strings.Split(strings.TrimRight(string(output), "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		added, err1 := strconv.Atoi(parts[0])
		deleted, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			continue // binary file ("-\t-\tpath")
		}
		path := numstatPath(parts[2])
		s := stats[path]
		s.added += added
		s.deleted += deleted
		stats[path] = s
	}
	return stats
}

func numstatPath(field string) string {
	if start := strings.Index(field, "{"); start != -1 {
		if end := strings.Index(field[start:], "}"); end != -1 {
			end += start
			prefix, mid, suffix := field[:start], field[start+1:end], field[end+1:]
			if arrow := strings.Index(mid, " => "); arrow != -1 {
				return prefix + mid[arrow+len(" => "):] + suffix
			}
		}
	}
	if arrow := strings.Index(field, " => "); arrow != -1 {
		return field[arrow+len(" => "):]
	}
	return field
}

// GetCommitsAheadBehind fetches from remote and returns how many commits
// the current branch is ahead and behind its upstream tracking branch.
func GetCommitsAheadBehind() (ahead int, behind int, err error) {
	// Fetch latest remote refs
	fetch := exec.Command("git", "fetch", "--quiet")
	fetch.Run() // ignore fetch errors (e.g. offline)

	cmd := exec.Command("git", "rev-list", "--count", "--left-right", "HEAD...@{upstream}")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("no upstream")
	}
	parts := strings.Fields(strings.TrimSpace(string(output)))
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected output")
	}
	fmt.Sscanf(parts[0], "%d", &ahead)
	fmt.Sscanf(parts[1], "%d", &behind)
	return ahead, behind, nil
}

var cachedDefaultBranch string

// GetDefaultBranch returns the default branch name (main or master), cached after first call.
func GetDefaultBranch() string {
	if cachedDefaultBranch != "" {
		return cachedDefaultBranch
	}
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	output, err := cmd.Output()
	if err == nil {
		ref := strings.TrimSpace(string(output))
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			cachedDefaultBranch = parts[len(parts)-1]
			return cachedDefaultBranch
		}
	}
	if exec.Command("git", "rev-parse", "--verify", "refs/heads/main").Run() == nil {
		cachedDefaultBranch = "main"
	} else {
		cachedDefaultBranch = "master"
	}
	return cachedDefaultBranch
}

// BranchFile represents a file changed in commits on this branch
type BranchFile struct {
	Status  string
	File    string
	Added   int
	Deleted int
}

// GetBranchDiffFiles returns files changed in commits on this branch
// since it diverged from the default branch.
func GetBranchDiffFiles() []BranchFile {
	defaultBranch := GetDefaultBranch()

	// Prefer the remote-tracking ref so the diff matches what a PR on the
	// remote would show, even if the local branch hasn't been pulled recently.
	baseRef := defaultBranch
	if out, err := exec.Command("git", "rev-parse", "--verify", "origin/"+defaultBranch).Output(); err == nil && strings.TrimSpace(string(out)) != "" {
		baseRef = "origin/" + defaultBranch
	}

	// Check if HEAD is the same ref as the default branch (handles detached HEAD too)
	headRev, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return nil
	}
	defaultRev, err := exec.Command("git", "rev-parse", baseRef).Output()
	if err != nil {
		return nil
	}
	if strings.TrimSpace(string(headRev)) == strings.TrimSpace(string(defaultRev)) {
		return nil
	}

	cmd := exec.Command("git", "merge-base", baseRef, "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	mergeBase := strings.TrimSpace(string(output))

	cmd = exec.Command("git", "diff", "--name-status", mergeBase, "HEAD")
	output, err = cmd.Output()
	if err != nil {
		return nil
	}

	var files []BranchFile
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		// Renamed/copied lines carry both the old and new path; show the
		// current (new) path.
		files = append(files, BranchFile{Status: parts[0], File: parts[len(parts)-1]})
	}

	stats := parseNumstat(runNumstat("git", "diff", "--numstat", mergeBase, "HEAD"))
	for i := range files {
		if s, ok := stats[files[i].File]; ok {
			files[i].Added, files[i].Deleted = s.added, s.deleted
		}
	}
	return files
}

func statusLabel(staged, unstaged byte) string {
	if staged == '?' && unstaged == '?' {
		return "untracked"
	}
	if staged == '!' && unstaged == '!' {
		return "ignored"
	}

	parts := []string{}

	switch staged {
	case 'M':
		parts = append(parts, "modified (staged)")
	case 'A':
		parts = append(parts, "added (staged)")
	case 'D':
		parts = append(parts, "deleted (staged)")
	case 'R':
		parts = append(parts, "renamed (staged)")
	case 'C':
		parts = append(parts, "copied (staged)")
	}

	switch unstaged {
	case 'M':
		parts = append(parts, "modified")
	case 'D':
		parts = append(parts, "deleted")
	}

	if len(parts) == 0 {
		return "changed"
	}
	return strings.Join(parts, ", ")
}
