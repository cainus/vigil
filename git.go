package main

import (
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
	cmd := exec.Command("git", "status", "--porcelain")
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
