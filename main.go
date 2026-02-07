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
	dir         string
	branch      string
	changes     []FileChange
	branchFiles []BranchFile
	ahead       int
	behind      int
	upstreamErr error
	viewport viewport.Model
	ready    bool
	width    int
	height   int
}

func initialModel() model {
	return model{
		branch:      GetCurrentBranch(),
		changes:     GetGitStatus(),
		branchFiles: GetBranchDiffFiles(),
	}
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
			m.branch = GetCurrentBranch()
			m.changes = GetGitStatus()
			m.branchFiles = GetBranchDiffFiles()
			m.viewport.SetContent(m.renderBody())
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
		m.branch = GetCurrentBranch()
		m.changes = GetGitStatus()
		m.branchFiles = GetBranchDiffFiles()
		m.viewport.SetContent(m.renderBody())
		cmds = append(cmds, tick(), tea.ClearScreen)

	case fetchTickMsg:
		m.ahead = msg.ahead
		m.behind = msg.behind
		m.upstreamErr = msg.err
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
	header.WriteString(pathStyle.Render(m.dir))
	header.WriteString("\n\n")
	header.WriteString("Branch: ")
	header.WriteString(branchStyle.Render(m.branch))
	if m.upstreamErr != nil {
		header.WriteString(helpStyle.Render(" (no upstream)"))
	} else if m.ahead == 0 && m.behind == 0 {
		header.WriteString(helpStyle.Render(" (up to date)"))
	} else {
		var parts []string
		if m.behind > 0 {
			parts = append(parts, fmt.Sprintf("%d behind", m.behind))
		}
		if m.ahead > 0 {
			parts = append(parts, fmt.Sprintf("%d ahead", m.ahead))
		}
		header.WriteString(helpStyle.Render(" (" + strings.Join(parts, ", ") + ")"))
	}
	header.WriteString("\n\n")

	// Footer
	footer := helpStyle.Render("\nScroll: ↑/↓/j/k  r: refresh  q: quit")

	return header.String() + m.viewport.View() + footer
}

func (m model) renderBody() string {
	var body strings.Builder
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
	// Check if we're in a git repo
	if !IsGitRepo() {
		fmt.Println("Error: Not a git repository")
		fmt.Println("Please run vigil from within a git repository.")
		os.Exit(1)
	}

	// Get current directory
	dir, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	// Create model
	m := initialModel()
	m.dir = dir

	// Run the program
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
