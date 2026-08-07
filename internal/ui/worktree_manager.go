package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// worktreeManagerRow is a single worktree entry shown in the manager.
type worktreeManagerRow struct {
	Path         string // Filesystem path to the worktree
	Branch       string // Branch checked out (may be empty for detached)
	SessionTitle string // Title of the associated agent-desk session (empty if orphaned)
	SessionID    string // ID of the associated session (empty if orphaned)
	IsMain       bool   // True for the main worktree (never removable)
	Orphaned     bool   // True if no session points here and it's not the main worktree
	Dirty        bool   // True if the worktree has uncommitted changes
}

// removable reports whether this row may be removed (orphans only, never main/active).
func (r worktreeManagerRow) removable() bool {
	return r.Orphaned && !r.IsMain && r.SessionID == ""
}

// WorktreeManager is an overlay that lists all git worktrees for the current
// repo, flags orphaned worktrees (no associated session), and lets the user
// remove orphans without dropping to the CLI. It mirrors WorktreeFinishDialog's
// structure (Show/Hide/IsVisible/SetSize/View) and is wired the same way.
type WorktreeManager struct {
	visible bool
	width   int
	height  int

	repoRoot string
	rows     []worktreeManagerRow
	cursor   int

	loading    bool // True while the async worktree scan is running
	confirming bool // True while awaiting y/n on a removal
	removing   bool // True while a removal is executing
	errorMsg   string
}

// NewWorktreeManager creates a new worktree manager overlay.
func NewWorktreeManager() *WorktreeManager {
	return &WorktreeManager{}
}

// Show opens the manager for the given repo root in a loading state. The caller
// is responsible for kicking off the async scan that calls SetRows.
func (m *WorktreeManager) Show(repoRoot string) {
	m.visible = true
	m.repoRoot = repoRoot
	m.rows = nil
	m.cursor = 0
	m.loading = true
	m.confirming = false
	m.removing = false
	m.errorMsg = ""
}

// Hide closes the manager and resets transient state.
func (m *WorktreeManager) Hide() {
	m.visible = false
	m.loading = false
	m.confirming = false
	m.removing = false
	m.errorMsg = ""
}

// IsVisible returns whether the manager is visible.
func (m *WorktreeManager) IsVisible() bool {
	return m.visible
}

// SetSize sets the overlay dimensions for centering.
func (m *WorktreeManager) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// RepoRoot returns the repo root this manager is scanning.
func (m *WorktreeManager) RepoRoot() string {
	return m.repoRoot
}

// SetRows populates the worktree list and exits the loading state.
func (m *WorktreeManager) SetRows(rows []worktreeManagerRow) {
	m.rows = rows
	m.loading = false
	m.removing = false
	m.confirming = false
	if m.cursor >= len(rows) {
		m.cursor = len(rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// SetError shows an error message and clears the busy states.
func (m *WorktreeManager) SetError(msg string) {
	m.errorMsg = msg
	m.loading = false
	m.removing = false
	m.confirming = false
}

// SetRemoving marks a removal as in-flight.
func (m *WorktreeManager) SetRemoving(removing bool) {
	m.removing = removing
}

// SelectedRow returns the currently highlighted row, or nil if none.
func (m *WorktreeManager) SelectedRow() *worktreeManagerRow {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return &m.rows[m.cursor]
}

// HandleKey processes a key event and returns an action for the caller:
// "close" to close the overlay, "remove" to remove the selected orphan,
// "reload" to rescan, or "" if fully handled here.
func (m *WorktreeManager) HandleKey(key string) (action string) {
	if m.loading || m.removing {
		return "" // Block input while busy
	}

	if m.confirming {
		switch key {
		case "y":
			m.confirming = false
			return "remove"
		case "n", "esc":
			m.confirming = false
			return ""
		}
		return ""
	}

	switch key {
	case "esc", "q":
		m.Hide()
		return "close"

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return ""

	case "down", "j":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
		return ""

	case "r":
		// Manual rescan
		m.errorMsg = ""
		m.loading = true
		return "reload"

	case "d", "x", "enter":
		// Request removal of the selected orphan
		row := m.SelectedRow()
		if row == nil {
			return ""
		}
		if !row.removable() {
			if row.IsMain {
				m.errorMsg = "Cannot remove the main worktree"
			} else {
				m.errorMsg = "Cannot remove a worktree with an active session"
			}
			return ""
		}
		m.errorMsg = ""
		m.confirming = true
		return ""
	}

	return ""
}

// View renders the overlay.
func (m *WorktreeManager) View() string {
	if !m.visible {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorCyan)
	dimStyle := lipgloss.NewStyle().Foreground(ColorComment)
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	activeStyle := lipgloss.NewStyle().Foreground(ColorGreen)
	orphanStyle := lipgloss.NewStyle().Foreground(ColorYellow)
	dirtyStyle := lipgloss.NewStyle().Foreground(ColorRed)
	errStyle := lipgloss.NewStyle().Foreground(ColorRed).Bold(true)
	branchStyle := lipgloss.NewStyle().Foreground(ColorCyan)

	// Responsive width
	dialogWidth := 72
	if m.width > 0 && m.width < dialogWidth+10 {
		dialogWidth = m.width - 10
		if dialogWidth < 40 {
			dialogWidth = 40
		}
	}

	boxBorder := ColorAccent
	if m.errorMsg != "" {
		boxBorder = ColorRed
	}
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(boxBorder).
		Background(ColorBg).
		Padding(1, 2).
		Width(dialogWidth)

	var b strings.Builder

	b.WriteString(titleStyle.Render("Worktree Manager"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(abbreviateHome(m.repoRoot)))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", dialogWidth-4))
	b.WriteString("\n\n")

	if m.loading {
		b.WriteString(dimStyle.Render("  Scanning worktrees..."))
		b.WriteString("\n")
		return m.place(boxStyle.Render(b.String()))
	}

	if len(m.rows) == 0 {
		b.WriteString(dimStyle.Render("  No worktrees found."))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Esc Close"))
		return m.place(boxStyle.Render(b.String()))
	}

	// Confirmation prompt replaces the footer when active.
	if m.confirming {
		row := m.SelectedRow()
		b.WriteString(m.renderRows(dialogWidth, selectedStyle, activeStyle, orphanStyle, dirtyStyle, branchStyle, dimStyle))
		b.WriteString("\n")
		warn := lipgloss.NewStyle().Foreground(ColorYellow).Bold(true)
		if row != nil {
			b.WriteString(warn.Render(fmt.Sprintf("  Remove orphaned worktree '%s'?", row.Branch)))
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("  " + abbreviateHome(row.Path)))
			b.WriteString("\n")
			if row.Dirty {
				b.WriteString(dirtyStyle.Render("  ⚠ has uncommitted changes (will be force-removed)"))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("y Remove | n Cancel"))
		return m.place(boxStyle.Render(b.String()))
	}

	if m.removing {
		b.WriteString(m.renderRows(dialogWidth, selectedStyle, activeStyle, orphanStyle, dirtyStyle, branchStyle, dimStyle))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("  Removing worktree..."))
		return m.place(boxStyle.Render(b.String()))
	}

	b.WriteString(m.renderRows(dialogWidth, selectedStyle, activeStyle, orphanStyle, dirtyStyle, branchStyle, dimStyle))

	if m.errorMsg != "" {
		b.WriteString("\n")
		b.WriteString(errStyle.Render("  " + m.errorMsg))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("j/k Move | d Remove orphan | r Rescan | Esc Close"))

	return m.place(boxStyle.Render(b.String()))
}

// renderRows renders the worktree table body.
func (m *WorktreeManager) renderRows(dialogWidth int, selectedStyle, activeStyle, orphanStyle, dirtyStyle, branchStyle, dimStyle lipgloss.Style) string {
	var b strings.Builder

	// Path column width: leave room for branch + status.
	pathWidth := dialogWidth - 4 - 22
	if pathWidth < 16 {
		pathWidth = 16
	}

	for i, row := range m.rows {
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}

		path := truncatePathLeft(abbreviateHome(row.Path), pathWidth)
		line := fmt.Sprintf("%s%-*s", prefix, pathWidth, path)
		if i == m.cursor {
			b.WriteString(selectedStyle.Render(line))
		} else {
			b.WriteString(line)
		}

		// Branch
		branch := row.Branch
		if branch == "" {
			branch = "(detached)"
		}
		b.WriteString(" ")
		b.WriteString(branchStyle.Render(truncStr(branch, 18)))

		// Status
		b.WriteString("\n")
		statusLine := "      "
		switch {
		case row.IsMain:
			statusLine += dimStyle.Render("main")
		case row.SessionID != "":
			statusLine += activeStyle.Render("● " + truncStr(row.SessionTitle, 30))
		case row.Orphaned:
			statusLine += orphanStyle.Render("orphaned")
		}
		if row.Dirty {
			statusLine += dirtyStyle.Render("  (dirty)")
		}
		b.WriteString(statusLine)
		b.WriteString("\n")
	}

	return b.String()
}

// place centers the rendered box in the available space.
func (m *WorktreeManager) place(content string) string {
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// abbreviateHome replaces the user's home dir prefix with ~ for display.
func abbreviateHome(path string) string {
	if path == "" {
		return path
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if path == home {
			return "~"
		}
		if strings.HasPrefix(path, home+string(os.PathSeparator)) {
			return "~" + path[len(home):]
		}
	}
	return path
}

// truncatePathLeft truncates a path from the left, prefixing "..." so the most
// specific (rightmost) segment stays visible.
func truncatePathLeft(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[len(s)-maxLen:]
	}
	return "..." + s[len(s)-(maxLen-3):]
}

// truncStr truncates a string from the right with an ellipsis.
func truncStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
