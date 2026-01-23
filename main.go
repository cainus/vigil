package main

import (
	"fmt"
	"os"
	"strings"

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

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)
)

// Messages
type fileChangeMsg struct{}
type tickMsg struct{}

// Model
type model struct {
	dir      string
	branch   string
	changes  []FileChange
	watcher  *Watcher
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

func (m model) Init() tea.Cmd {
	return tea.Batch(
		watchForChanges(m.watcher),
		tea.EnterAltScreen,
	)
}

func watchForChanges(w *Watcher) tea.Cmd {
	return func() tea.Msg {
		if w == nil {
			return nil
		}
		<-w.Events
		return fileChangeMsg{}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			if m.watcher != nil {
				m.watcher.Close()
			}
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

	case fileChangeMsg:
		m.branch = GetCurrentBranch()
		m.changes = GetGitStatus()
		cmds = append(cmds, watchForChanges(m.watcher))
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	var b strings.Builder

	// ASCII Art Title
	b.WriteString(asciiStyle.Render(asciiArt))
	b.WriteString("\n")

	// Directory path
	b.WriteString(pathStyle.Render(m.dir))
	b.WriteString("\n\n")

	// Branch
	b.WriteString("Branch: ")
	b.WriteString(branchStyle.Render(m.branch))
	b.WriteString("\n\n")

	// Changed files
	if len(m.changes) == 0 {
		b.WriteString(helpStyle.Render("No changes detected"))
	} else {
		b.WriteString("Changed Files:\n")
		for _, change := range m.changes {
			status := formatStatus(change.Status)
			file := fileStyle.Render(change.File)
			b.WriteString(fmt.Sprintf("  %s %s\n", status, file))
		}
	}

	// Update viewport content
	m.viewport.SetContent(b.String())

	// Footer
	footer := helpStyle.Render("\nWatching for changes... Press 'q' to quit")

	return m.viewport.View() + footer
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

	// Create watcher
	watcher, err := NewWatcher(dir)
	if err != nil {
		fmt.Printf("Error creating watcher: %v\n", err)
		os.Exit(1)
	}
	defer watcher.Close()

	// Create model with watcher
	m := initialModel()
	m.dir = dir
	m.watcher = watcher

	// Run the program
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
