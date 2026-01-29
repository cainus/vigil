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

// Model
type model struct {
	dir      string
	branch   string
	changes  []FileChange
	viewport viewport.Model
	ready    bool
	width    int
	height   int
}

func initialModel() model {
	return model{
		branch:  GetCurrentBranch(),
		changes: GetGitStatus(),
	}
}

func tick() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tick(), tea.EnterAltScreen)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		headerHeight := 8 // ASCII art + path + branch + spacing
		footerHeight := 2 // Help text
		verticalMargin := headerHeight + footerHeight

		if !m.ready {
			m.viewport = viewport.New(msg.Width-4, msg.Height-verticalMargin)
			m.viewport.YPosition = headerHeight
			m.ready = true
		} else {
			m.viewport.Width = msg.Width - 4
			m.viewport.Height = msg.Height - verticalMargin
		}

	case tickMsg:
		m.branch = GetCurrentBranch()
		m.changes = GetGitStatus()
		cmds = append(cmds, tick())
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

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
	header.WriteString("\n\n")

	// File list (rendered inside viewport for scrolling)
	var body strings.Builder
	if len(m.changes) == 0 {
		body.WriteString(helpStyle.Render("No changes detected"))
	} else {
		body.WriteString("Changed Files:\n")
		for _, change := range m.changes {
			status := formatStatus(change.Status)
			file := fileStyle.Render(change.File)
			body.WriteString(fmt.Sprintf("  %s %s\n", status, file))
		}
	}

	m.viewport.SetContent(body.String())

	// Footer
	footer := helpStyle.Render("\nWatching for changes... Press 'q' to quit")

	return header.String() + m.viewport.View() + footer
}

func formatStatus(status string) string {
	// Pad status to 2 characters for alignment
	padded := fmt.Sprintf("%-2s", status)

	switch {
	case strings.Contains(status, "M"):
		return statusModified.Render(padded)
	case strings.Contains(status, "A"):
		return statusAdded.Render(padded)
	case strings.Contains(status, "D"):
		return statusDeleted.Render(padded)
	case strings.Contains(status, "R"):
		return statusRenamed.Render(padded)
	case status == "??":
		return statusUntracked.Render(padded)
	default:
		return padded
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
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
