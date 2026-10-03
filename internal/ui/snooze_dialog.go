package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/millwright-software/agent-desk/internal/session"
)

// snoozeSetMsg is sent when the user picks a wake time.
type snoozeSetMsg struct {
	instanceID string
	until      time.Time
}

// SnoozeDialog is the s-key picker: the preset rows from
// session.SnoozePresets plus a free-text row that takes anything
// session.ParseSnooze reads ("2h", "7am", "mon 9am", "10/12 11:00").
type SnoozeDialog struct {
	visible    bool
	width      int
	height     int
	cursor     int // 0..len(presets)-1 = preset row; len(presets) = custom row
	instanceID string
	title      string
	input      textinput.Model
	err        string
	now        time.Time
}

// NewSnoozeDialog creates the dialog.
func NewSnoozeDialog() *SnoozeDialog {
	ti := textinput.New()
	ti.Placeholder = "2h · 7am · mon 9am · 10/12 11:00"
	ti.CharLimit = 32
	ti.Width = 30
	return &SnoozeDialog{input: ti}
}

// Show opens the dialog for a session.
func (d *SnoozeDialog) Show(instanceID, title string) {
	d.visible = true
	d.instanceID = instanceID
	d.title = title
	d.cursor = 0
	d.err = ""
	d.now = time.Now()
	d.input.SetValue("")
	d.input.Blur()
}

// Hide closes the dialog.
func (d *SnoozeDialog) Hide() {
	d.visible = false
	d.input.Blur()
}

// IsVisible reports whether the dialog is shown.
func (d *SnoozeDialog) IsVisible() bool { return d.visible }

// SetSize updates dialog dimensions.
func (d *SnoozeDialog) SetSize(width, height int) {
	d.width = width
	d.height = height
}

func (d *SnoozeDialog) customRow() int { return len(session.SnoozePresets) }

func (d *SnoozeDialog) setCursor(c int) {
	d.cursor = c
	d.err = ""
	if c == d.customRow() {
		d.input.Focus()
	} else {
		d.input.Blur()
	}
}

// Update handles key input. Returns a snoozeSetMsg command on Enter.
func (d *SnoozeDialog) Update(msg tea.KeyMsg) (*SnoozeDialog, tea.Cmd) {
	if !d.visible {
		return d, nil
	}
	onCustom := d.cursor == d.customRow()
	switch msg.String() {
	case "esc":
		d.Hide()
		return d, nil
	case "up", "ctrl+p":
		if d.cursor > 0 {
			d.setCursor(d.cursor - 1)
		}
		return d, nil
	case "down", "ctrl+n", "tab":
		if d.cursor < d.customRow() {
			d.setCursor(d.cursor + 1)
		}
		return d, nil
	case "k":
		if !onCustom {
			if d.cursor > 0 {
				d.setCursor(d.cursor - 1)
			}
			return d, nil
		}
	case "j":
		if !onCustom {
			d.setCursor(d.cursor + 1)
			return d, nil
		}
	case "enter":
		spec := ""
		if onCustom {
			spec = d.input.Value()
		} else {
			spec = session.SnoozePresets[d.cursor].Spec
		}
		until, err := session.ParseSnooze(spec, time.Now())
		if err != nil {
			d.err = err.Error()
			if !onCustom {
				d.setCursor(d.customRow())
				d.input.SetValue(spec)
			}
			return d, nil
		}
		instanceID := d.instanceID
		d.Hide()
		return d, func() tea.Msg {
			return snoozeSetMsg{instanceID: instanceID, until: until}
		}
	}
	if onCustom {
		var cmd tea.Cmd
		d.input, cmd = d.input.Update(msg)
		d.err = ""
		return d, cmd
	}
	// Any other printable key on a preset row starts a custom entry.
	if len(msg.Runes) == 1 && msg.Type == tea.KeyRunes {
		d.setCursor(d.customRow())
		var cmd tea.Cmd
		d.input, cmd = d.input.Update(msg)
		return d, cmd
	}
	return d, nil
}

// View renders the picker with each preset's resolved wake time.
func (d *SnoozeDialog) View() string {
	if !d.visible {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorCyan)
	dimStyle := lipgloss.NewStyle().Foreground(ColorComment)
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	timeStyle := lipgloss.NewStyle().Foreground(ColorTextDim)
	errStyle := lipgloss.NewStyle().Foreground(ColorRed)

	dialogWidth := 48
	if d.width > 0 && d.width < dialogWidth+10 {
		dialogWidth = d.width - 10
		if dialogWidth < 34 {
			dialogWidth = 34
		}
	}

	var content strings.Builder
	content.WriteString(titleStyle.Render("Snooze session"))
	content.WriteString(dimStyle.Render("          [Esc] Cancel"))
	content.WriteString("\n")
	content.WriteString(strings.Repeat("-", dialogWidth-4))
	content.WriteString("\n")
	name := d.title
	if len(name) > dialogWidth-8 {
		name = name[:dialogWidth-11] + "..."
	}
	content.WriteString(dimStyle.Render(name))
	content.WriteString("\n\n")

	now := time.Now()
	for i, p := range session.SnoozePresets {
		prefix := "  "
		label := p.Label
		when := ""
		if t, err := session.ParseSnooze(p.Spec, now); err == nil {
			when = t.Format("Mon 3:04pm")
		}
		line := prefix
		if i == d.cursor {
			line = "> " + selectedStyle.Render(label)
		} else {
			line += label
		}
		pad := 20 - len(label)
		if pad < 1 {
			pad = 1
		}
		line += strings.Repeat(" ", pad) + timeStyle.Render(when)
		content.WriteString(line)
		content.WriteString("\n")
	}

	// Custom row
	if d.cursor == d.customRow() {
		content.WriteString("> " + selectedStyle.Render("At:") + " " + d.input.View())
	} else {
		content.WriteString("  At: " + dimStyle.Render("type a time"))
	}
	content.WriteString("\n")

	if d.err != "" {
		content.WriteString("\n")
		content.WriteString(errStyle.Render(d.err))
	}

	content.WriteString("\n")
	content.WriteString(dimStyle.Render("↑/↓ Pick  Enter Snooze  Esc Cancel"))

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
