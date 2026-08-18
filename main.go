package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const asciiArt = `
 █░█ █ █▀▀ █ █░░
 ▀▄▀ █ █▄█ █ █▄▄
`

// Styles
var (
	asciiStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))

	pathStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	branchStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("42"))

	statusModified = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	statusAdded = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	statusDeleted = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	statusUntracked = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	statusRenamed = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))

	fileStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

// Messages
type tickMsg struct{}
type fetchTickMsg struct {
	ahead  int
	behind int
	err    error
}

// Model
type model struct {
	dir           string
	repoName      string
	isGitRepo     bool
	branch        string
	changes       []FileChange
	branchFiles   []BranchFile
	statusErr     error
	files         []string // filesystem files when not in a git repo
	ahead         int
	behind        int
	upstreamErr   error
	upstreamSeen  bool
	upstreamStale bool
	viewport      viewport.Model
	ready         bool
	width         int
	height        int
}

func initialModel(isGitRepo bool, dir string) model {
	if !isGitRepo {
		return model{
			isGitRepo: false,
			dir:       dir,
			files:     ListFiles(dir),
		}
	}
	changes, statusErr := GetGitStatusWithError()
	return model{
		isGitRepo:   true,
		dir:         dir,
		repoName:    GetRepoName(),
		branch:      GetCurrentBranch(),
		changes:     changes,
		statusErr:   statusErr,
		branchFiles: GetBranchDiffFiles(),
	}
}

// refresh re-reads filesystem / git state. Returns true if the directory
// transitioned from non-git to git on this call.
func (m *model) refresh() bool {
	wasGit := m.isGitRepo
	m.isGitRepo = IsGitRepo()
	if m.isGitRepo {
		if !wasGit {
			cachedDefaultBranch = ""
			m.repoName = GetRepoName()
			m.files = nil
		}
		m.branch = GetCurrentBranch()
		m.changes, m.statusErr = GetGitStatusWithError()
		m.branchFiles = GetBranchDiffFiles()
	} else {
		if wasGit {
			m.repoName = ""
			m.branch = ""
			m.changes = nil
			m.branchFiles = nil
			m.statusErr = nil
			m.ahead = 0
			m.behind = 0
			m.upstreamErr = nil
			m.upstreamSeen = false
			m.upstreamStale = false
		}
		m.files = ListFiles(m.dir)
	}
	return !wasGit && m.isGitRepo
}

func (m *model) refreshAndMarkUpstreamStale() bool {
	becameGit := m.refresh()
	if m.isGitRepo && m.upstreamSeen {
		m.upstreamStale = true
	}
	return becameGit
}

func tick() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

func fetchUpstream() tea.Msg {
	ahead, behind, err := GetCommitsAheadBehind()
	return fetchTickMsg{ahead: ahead, behind: behind, err: err}
}

func scheduleFetch() tea.Cmd {
	return tea.Tick(2*time.Minute, func(t time.Time) tea.Msg {
		return fetchUpstream()
	})
}

func (m model) Init() tea.Cmd {
	if !m.isGitRepo {
		return tea.Batch(tick(), tea.EnterAltScreen)
	}
	return tea.Batch(tick(), tea.EnterAltScreen, fetchUpstream)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "up", "k":
			m.viewport.LineUp(1)
		case "down", "j":
			m.viewport.LineDown(1)
		case "pgup":
			m.viewport.HalfViewUp()
		case "pgdown":
			m.viewport.HalfViewDown()
		case "r":
			m.refreshAndMarkUpstreamStale()
			m.viewport.SetContent(m.renderBody())
			if m.isGitRepo {
				return m, tea.Batch(tea.ClearScreen, fetchUpstream)
			}
			return m, tea.ClearScreen
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		headerHeight := 8 // ASCII art + path + branch + spacing
		footerHeight := 2 // Help text
		verticalMargin := headerHeight + footerHeight

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMargin)
			m.viewport.SetContent(m.renderBody())
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMargin
			m.viewport.SetContent(m.renderBody())
		}

	case tickMsg:
		becameGit := m.refresh()
		m.viewport.SetContent(m.renderBody())
		cmds = append(cmds, tick(), tea.ClearScreen)
		if becameGit {
			cmds = append(cmds, fetchUpstream)
		}

	case fetchTickMsg:
		m.ahead = msg.ahead
		m.behind = msg.behind
		m.upstreamErr = msg.err
		m.upstreamSeen = true
		m.upstreamStale = false
		cmds = append(cmds, scheduleFetch())
	}

	if m.ready {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	// Header (rendered outside viewport)
	var header strings.Builder
	header.WriteString(asciiStyle.Render(asciiArt))
	header.WriteString("\n")
	if m.isGitRepo {
		header.WriteString(branchStyle.Render(m.repoName))
		header.WriteString(pathStyle.Render(" " + m.dir))
		header.WriteString("\n\n")
		header.WriteString("Branch: ")
		header.WriteString(branchStyle.Render(m.branch))
		if m.upstreamErr != nil {
			header.WriteString(helpStyle.Render(" (no upstream)"))
		} else if !m.upstreamSeen {
			header.WriteString(helpStyle.Render(" (checking upstream)"))
		} else if m.ahead == 0 && m.behind == 0 {
			status := "up to date"
			if m.upstreamStale {
				status += ", stale upstream status"
			}
			header.WriteString(helpStyle.Render(" (" + status + ")"))
		} else {
			var parts []string
			if m.behind > 0 {
				parts = append(parts, fmt.Sprintf("%d behind", m.behind))
			}
			if m.ahead > 0 {
				parts = append(parts, fmt.Sprintf("%d ahead", m.ahead))
			}
			if m.upstreamStale {
				parts = append(parts, "stale upstream status")
			}
			header.WriteString(helpStyle.Render(" (" + strings.Join(parts, ", ") + ")"))
		}
	} else {
		header.WriteString(pathStyle.Render(m.dir))
		header.WriteString("\n\n")
		header.WriteString(helpStyle.Render("Not a git repository"))
	}
	header.WriteString("\n\n")

	// Footer
	footer := helpStyle.Render("\nScroll: ↑/↓/j/k  r: refresh  q: quit")

	return header.String() + m.viewport.View() + footer
}

func (m model) renderBody() string {
	var body strings.Builder
	if !m.isGitRepo {
		if len(m.files) == 0 {
			body.WriteString(helpStyle.Render("Empty directory"))
		} else {
			body.WriteString("Files:\n")
			for _, f := range m.files {
				if strings.HasSuffix(f, "/") {
					body.WriteString(fmt.Sprintf("  %s\n", branchStyle.Render(f)))
				} else {
					body.WriteString(fmt.Sprintf("  %s\n", fileStyle.Render(f)))
				}
			}
		}
		return body.String()
	}
	if m.statusErr != nil {
		body.WriteString(statusDeleted.Render("Unable to read git status"))
		body.WriteString(helpStyle.Render(": " + m.statusErr.Error()))
		if len(m.branchFiles) > 0 {
			body.WriteString("\n\n")
		} else {
			return body.String()
		}
	}
	if len(m.changes) == 0 && len(m.branchFiles) == 0 {
		body.WriteString(helpStyle.Render("No changes detected"))
	} else {
		if len(m.changes) > 0 {
			body.WriteString("Changed Files:\n")
			for _, change := range m.changes {
				label := formatLabel(change)
				file := fileStyle.Render(change.File)
				body.WriteString(fmt.Sprintf("  %s  %s\n", label, file))
			}
		}
		if len(m.branchFiles) > 0 {
			if len(m.changes) > 0 {
				body.WriteString("\n")
			}
			body.WriteString("Branch Files:\n")
			for _, bf := range m.branchFiles {
				label := fmt.Sprintf("%-12s", branchFileLabel(bf.Status))
				styled := statusModified.Render(label)
				if bf.Status == "A" {
					styled = statusAdded.Render(label)
				} else if bf.Status == "D" {
					styled = statusDeleted.Render(label)
				} else if strings.HasPrefix(bf.Status, "R") {
					styled = statusRenamed.Render(label)
				}
				body.WriteString(fmt.Sprintf("  %s  %s\n", styled, fileStyle.Render(bf.File)))
			}
		}
	}
	return body.String()
}

func formatLabel(c FileChange) string {
	padded := fmt.Sprintf("%-12s", c.Label)

	if c.Staged == '?' {
		return statusUntracked.Render(padded)
	}
	if c.Staged == 'D' || c.Unstaged == 'D' {
		return statusDeleted.Render(padded)
	}
	if c.Staged == 'A' {
		return statusAdded.Render(padded)
	}
	if c.Staged == 'R' {
		return statusRenamed.Render(padded)
	}
	if c.Staged != ' ' && c.Staged != 0 {
		return statusAdded.Render(padded) // staged changes in green
	}
	return statusModified.Render(padded)
}

func branchFileLabel(status string) string {
	switch {
	case status == "A":
		return "added"
	case status == "D":
		return "deleted"
	case status == "M":
		return "modified"
	case strings.HasPrefix(status, "R"):
		return "renamed"
	case strings.HasPrefix(status, "C"):
		return "copied"
	default:
		return "changed"
	}
}

func main() {
	// Get current directory
	dir, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	// Create model
	m := initialModel(IsGitRepo(), dir)

	// Run the program
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
