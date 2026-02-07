package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// FileChange represents a changed file in git status
type FileChange struct {
	Staged   byte // first column: staged status
	Unstaged byte // second column: unstaged status
	Label    string
	File     string
}

// IsGitRepo checks if the current directory is inside a git repository
func IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	err := cmd.Run()
	return err == nil
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

// GetGitStatus returns a list of changed files from git status
func GetGitStatus() []FileChange {
	cmd := exec.Command("git", "status", "--porcelain", "-uall")
	output, err := cmd.Output()
	if err != nil {
		return nil
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

		label := statusLabel(staged, unstaged)
		changes = append(changes, FileChange{
			Staged:   staged,
			Unstaged: unstaged,
			Label:    label,
			File:     file,
		})
	}
	return changes
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
	Status string
	File   string
}

// GetBranchDiffFiles returns files changed in commits on this branch
// since it diverged from the default branch.
func GetBranchDiffFiles() []BranchFile {
	defaultBranch := GetDefaultBranch()

	// Check if HEAD is the same ref as the default branch (handles detached HEAD too)
	headRev, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return nil
	}
	defaultRev, err := exec.Command("git", "rev-parse", defaultBranch).Output()
	if err != nil {
		return nil
	}
	if strings.TrimSpace(string(headRev)) == strings.TrimSpace(string(defaultRev)) {
		return nil
	}

	cmd := exec.Command("git", "merge-base", defaultBranch, "HEAD")
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
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		files = append(files, BranchFile{Status: parts[0], File: parts[1]})
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
