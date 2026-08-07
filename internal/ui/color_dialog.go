package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/millwright-software/agent-desk/internal/session"
)

// colorSchemeSelectedMsg is sent when the user picks a color scheme.
type colorSchemeSelectedMsg struct {
	scheme     string
	instanceID string
}

// ColorSchemeDialog lets the user pick a per-session color scheme from the
// preset palette. The scheme applies to the attached tmux window and the
// agent-desk preview pane.
type ColorSchemeDialog struct {
	visible    bool
	width      int
	height     int
	cursor     int
	instanceID string
	current    string // currently applied scheme name ("" = Default)
}

// NewColorSchemeDialog creates the dialog.
func NewColorSchemeDialog() *ColorSchemeDialog {
	return &ColorSchemeDialog{}
}

// Show opens the dialog for a session, positioning the cursor on its current scheme.
func (d *ColorSchemeDialog) Show(instanceID, current string) {
	d.visible = true
	d.instanceID = instanceID
	d.current = current
	d.cursor = 0
	resolved := session.ColorSchemeByName(current)
	for i, cs := range session.ColorSchemes {
		if cs.Name == resolved.Name {
			d.cursor = i
			break
		}
	}
}

// Hide closes the dialog.
func (d *ColorSchemeDialog) Hide() { d.visible = false }

// IsVisible reports whether the dialog is shown.
func (d *ColorSchemeDialog) IsVisible() bool { return d.visible }

// SetSize updates dialog dimensions.
func (d *ColorSchemeDialog) SetSize(width, height int) {
	d.width = width
	d.height = height
}

// Update handles key input. Returns a colorSchemeSelectedMsg command on Enter.
func (d *ColorSchemeDialog) Update(msg tea.KeyMsg) (*ColorSchemeDialog, tea.Cmd) {
	if !d.visible {
		return d, nil
	}
	switch msg.String() {
	case "esc":
		d.Hide()
		return d, nil
	case "up", "k":
		if d.cursor > 0 {
			d.cursor--
		}
	case "down", "j":
		if d.cursor < len(session.ColorSchemes)-1 {
			d.cursor++
		}
	case "enter":
		if d.cursor >= 0 && d.cursor < len(session.ColorSchemes) {
			selected := session.ColorSchemes[d.cursor].Name
			instanceID := d.instanceID
			d.Hide()
			return d, func() tea.Msg {
				return colorSchemeSelectedMsg{scheme: selected, instanceID: instanceID}
			}
		}
	}
	return d, nil
}

// View renders the dialog with a live color swatch per preset.
func (d *ColorSchemeDialog) View() string {
	if !d.visible {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorCyan)
	dimStyle := lipgloss.NewStyle().Foreground(ColorComment)
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	currentStyle := lipgloss.NewStyle().Foreground(ColorGreen)

	dialogWidth := 46
	if d.width > 0 && d.width < dialogWidth+10 {
		dialogWidth = d.width - 10
		if dialogWidth < 32 {
			dialogWidth = 32
		}
	}

	var content strings.Builder
	content.WriteString(titleStyle.Render("Session Color Scheme"))
	content.WriteString(dimStyle.Render("        [Esc] Cancel"))
	content.WriteString("\n")
	content.WriteString(strings.Repeat("-", dialogWidth-4))
	content.WriteString("\n\n")

	resolvedCurrent := session.ColorSchemeByName(d.current).Name

	// Scrolling window
	maxVisible := 15
	if d.height > 0 {
		maxVisible = d.height/2 - 6
		if maxVisible < 6 {
			maxVisible = 6
		}
	}
	start := 0
	if d.cursor >= maxVisible {
		start = d.cursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(session.ColorSchemes) {
		end = len(session.ColorSchemes)
	}

	for i := start; i < end; i++ {
		cs := session.ColorSchemes[i]

		prefix := "  "
		if i == d.cursor {
			prefix = "> "
		}

		// Swatch: blocks in the scheme's background with an accent bar, so the
		// actual pane background and accent are previewed inline.
		swatch := dimStyle.Render("──")
		if cs.Bg != "" {
			swatch = lipgloss.NewStyle().
				Background(lipgloss.Color(cs.Bg)).
				Foreground(lipgloss.Color(cs.Accent)).
				Render(" ▌█ ")
		}

		label := cs.Name
		if cs.IsDefault() {
			label += " (default)"
		}
		if cs.Name == resolvedCurrent {
			label += " (current)"
		}

		line := prefix + swatch + " "
		if i == d.cursor {
			line += selectedStyle.Render(label)
		} else if cs.Name == resolvedCurrent {
			line += currentStyle.Render(label)
		} else {
			line += label
		}
		content.WriteString(line)
		content.WriteString("\n")
	}

	content.WriteString("\n")
	content.WriteString(dimStyle.Render("j/k Navigate  Enter Apply  Esc Cancel"))

	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorCyan).
		Background(ColorBg).
		Padding(1, 2).
		Width(dialogWidth)

	return lipgloss.Place(
		d.width, d.height,
		lipgloss.Center, lipgloss.Center,
		dialogStyle.Render(content.String()),
	)
}
