package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const headerHeight = 8 // ASCII art + path + branch + spacing
const toastDuration = 1500 * time.Millisecond

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
type toastClearMsg struct{ gen int }

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
	headRevision  string
	ahead         int
	behind        int
	upstreamErr   error
	upstreamSeen  bool
	upstreamStale bool
	viewport      viewport.Model
	ready         bool
	width         int
	height        int

	bodyRows []bodyRow // structured body content, used for rendering and mouse selection

	mouseDown                bool // left button held: a drag selection is in progress
	selStartRow, selStartCol int
	selEndRow, selEndCol     int

	toastMsg string
	toastGen int
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
		isGitRepo:    true,
		dir:          dir,
		repoName:     GetRepoName(),
		branch:       GetCurrentBranch(),
		headRevision: GetHeadRevision(),
		changes:      changes,
		statusErr:    statusErr,
		branchFiles:  GetBranchDiffFiles(),
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
		previousHead := m.headRevision
		m.branch = GetCurrentBranch()
		m.headRevision = GetHeadRevision()
		m.changes, m.statusErr = GetGitStatusWithError()
		m.branchFiles = GetBranchDiffFiles()
		if previousHead != "" && m.headRevision != "" && previousHead != m.headRevision && m.upstreamSeen {
			m.upstreamStale = true
		}
	} else {
		if wasGit {
			m.repoName = ""
			m.branch = ""
			m.headRevision = ""
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

// handleMouse processes a mouse event, tracking a left-button drag as a
// text selection and copying it to the clipboard on release.
func (m *model) handleMouse(msg tea.MouseMsg, cmds *[]tea.Cmd) {
	switch msg.Action {
	case tea.MouseActionPress:
		if msg.Button != tea.MouseButtonLeft {
			return
		}
		row, col := m.screenToContent(msg.X, msg.Y)
		m.mouseDown = true
		m.selStartRow, m.selStartCol = row, col
		m.selEndRow, m.selEndCol = row, col
		m.setBody()

	case tea.MouseActionMotion:
		if !m.mouseDown {
			return
		}
		row, col := m.screenToContent(msg.X, msg.Y)
		m.selEndRow, m.selEndCol = row, col
		m.setBody()

	case tea.MouseActionRelease:
		if !m.mouseDown {
			return
		}
		m.mouseDown = false
		sel := normalizeSelection(m.selStartRow, m.selStartCol, m.selEndRow, m.selEndCol)
		text := selectedText(m.bodyRows, sel)
		if text != "" && clipboard.WriteAll(text) == nil {
			m.toastGen++
			gen := m.toastGen
			m.toastMsg = "Copied to clipboard"
			*cmds = append(*cmds, tea.Tick(toastDuration, func(time.Time) tea.Msg {
				return toastClearMsg{gen: gen}
			}))
		}
		m.setBody()
	}
}

// screenToContent maps a terminal cell coordinate to a row/column in
// m.bodyRows, clamping to the nearest valid row so drags that leave the
// viewport still extend the selection sensibly.
func (m model) screenToContent(x, y int) (row, col int) {
	row = y - headerHeight + m.viewport.YOffset
	row = clampInt(row, 0, len(m.bodyRows)-1)
	if row < 0 {
		row = 0
	}
	col = max(0, x)
	return row, col
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
			m.setBody()
			if m.isGitRepo {
				return m, tea.Batch(tea.ClearScreen, fetchUpstream)
			}
			return m, tea.ClearScreen
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		footerHeight := 2 // Help text
		verticalMargin := headerHeight + footerHeight

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMargin)
			m.setBody()
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMargin
			m.setBody()
		}

	case tickMsg:
		wasUpstreamStale := m.upstreamStale
		becameGit := m.refresh()
		m.setBody()
		cmds = append(cmds, tick(), tea.ClearScreen)
		if becameGit || (!wasUpstreamStale && m.upstreamStale) {
			cmds = append(cmds, fetchUpstream)
		}

	case fetchTickMsg:
		m.ahead = msg.ahead
		m.behind = msg.behind
		m.upstreamErr = msg.err
		m.upstreamSeen = true
		m.upstreamStale = false
		cmds = append(cmds, scheduleFetch())

	case tea.MouseMsg:
		m.handleMouse(msg, &cmds)

	case toastClearMsg:
		if msg.gen == m.toastGen {
			m.toastMsg = ""
		}
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
	footerText := "Scroll: ↑/↓/j/k  r: refresh  q: quit"
	if m.toastMsg != "" {
		footerText = m.toastMsg
	}
	footer := helpStyle.Render("\n" + footerText)

	return header.String() + m.viewport.View() + footer
}

// setBody rebuilds m.bodyRows from current state and pushes the rendered
// result into the viewport, preserving any active mouse selection highlight.
func (m *model) setBody() {
	m.bodyRows = m.buildBodyRows()
	var sel *selection
	if m.mouseDown {
		s := normalizeSelection(m.selStartRow, m.selStartCol, m.selEndRow, m.selEndCol)
		sel = &s
	}
	m.viewport.SetContent(renderRows(m.bodyRows, sel))
}

// renderBody returns the unhighlighted body as plain rendered text, mainly
// for tests; the live view renders via setBody so it can show a selection.
func (m model) renderBody() string {
	return renderRows(m.buildBodyRows(), nil)
}

func (m model) buildBodyRows() []bodyRow {
	var rows []bodyRow
	if !m.isGitRepo {
		if len(m.files) == 0 {
			rows = append(rows, newRow(styledSeg("Empty directory", helpStyle)))
		} else {
			rows = append(rows, newRow(plainSeg("Files:")))
			for _, f := range m.files {
				style := fileStyle
				if strings.HasSuffix(f, "/") {
					style = branchStyle
				}
				rows = append(rows, newRow(plainSeg("  "), styledSeg(f, style)))
			}
		}
		return rows
	}
	if m.statusErr != nil {
		rows = append(rows, newRow(
			styledSeg("Unable to read git status", statusDeleted),
			styledSeg(": "+m.statusErr.Error(), helpStyle),
		))
		if len(m.branchFiles) == 0 {
			return rows
		}
		rows = append(rows, newRow())
	}
	if len(m.changes) == 0 && len(m.branchFiles) == 0 {
		rows = append(rows, newRow(styledSeg("No changes detected", helpStyle)))
		return rows
	}
	if len(m.changes) > 0 {
		rows = append(rows, newRow(plainSeg("Changed Files:")))
		for _, change := range m.changes {
			text, style := formatLabel(change)
			rows = append(rows, newRow(
				plainSeg("  "),
				styledSeg(fmt.Sprintf("%-12s", text), style),
				plainSeg("  "),
				styledSeg(change.File, fileStyle),
			))
		}
	}
	if len(m.branchFiles) > 0 {
		if len(m.changes) > 0 {
			rows = append(rows, newRow())
		}
		rows = append(rows, newRow(plainSeg("Branch Files:")))
		for _, bf := range m.branchFiles {
			text, style := branchFileLabel(bf.Status)
			rows = append(rows, newRow(
				plainSeg("  "),
				styledSeg(fmt.Sprintf("%-12s", text), style),
				plainSeg("  "),
				styledSeg(bf.File, fileStyle),
			))
		}
	}
	return rows
}

func formatLabel(c FileChange) (string, lipgloss.Style) {
	if c.Staged == '?' {
		return c.Label, statusUntracked
	}
	if c.Staged == 'D' || c.Unstaged == 'D' {
		return c.Label, statusDeleted
	}
	if c.Staged == 'A' {
		return c.Label, statusAdded
	}
	if c.Staged == 'R' {
		return c.Label, statusRenamed
	}
	if c.Staged != ' ' && c.Staged != 0 {
		return c.Label, statusAdded // staged changes in green
	}
	return c.Label, statusModified
}

func branchFileLabel(status string) (string, lipgloss.Style) {
	switch {
	case status == "A":
		return "added", statusAdded
	case status == "D":
		return "deleted", statusDeleted
	case status == "M":
		return "modified", statusModified
	case strings.HasPrefix(status, "R"):
		return "renamed", statusRenamed
	case strings.HasPrefix(status, "C"):
		return "copied", statusModified
	default:
		return "changed", statusModified
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
