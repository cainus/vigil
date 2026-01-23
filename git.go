package main

import (
	"os/exec"
	"strings"
)

// FileChange represents a changed file in git status
type FileChange struct {
	Status string
	File   string
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
	if err != nil {
		// Might be in detached HEAD state
		cmd = exec.Command("git", "rev-parse", "--short", "HEAD")
		output, err = cmd.Output()
		if err != nil {
			return "unknown"
		}
		return "(detached) " + strings.TrimSpace(string(output))
	}
	branch := strings.TrimSpace(string(output))
	if branch == "" {
		return "unknown"
	}
	return branch
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
		if len(line) < 3 {
			continue
		}
		status := strings.TrimSpace(line[:2])
		file := line[3:]
		changes = append(changes, FileChange{
			Status: status,
			File:   file,
		})
	}
	return changes
}
