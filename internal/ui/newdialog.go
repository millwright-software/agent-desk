package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/millwright-software/agent-desk/internal/git"
	"github.com/millwright-software/agent-desk/internal/session"
)

// NewDialog represents the new session creation dialog
type NewDialog struct {
	nameInput       textinput.Model
	pathInput       textinput.Model
	commandInput    textinput.Model
	claudeOptions   *ClaudeOptionsPanel // Claude-specific options (concrete for value extraction)
	geminiOptions   *YoloOptionsPanel   // Gemini YOLO panel (concrete for value extraction)
	codexOptions    *YoloOptionsPanel   // Codex YOLO panel (concrete for value extraction)
	toolOptions     OptionsPanel        // Currently active tool options panel (nil if none)
	focusIndex      int                 // 0=name, 1=path, 2=command, 3+=options
	width           int
	height          int
	visible         bool
	presetCommands  []string
	commandCursor   int
	parentGroupPath string
	parentGroupName string
	// Path selection (unified filterable dropdown)
	pathSuggestions     []string       // recent project paths sorted by recency
	dropdownCursor      int            // -1 = no selection, 0+ = highlighted item
	dropdownItems       []dropdownItem // filtered: recent matches first, then fs completions
	cachedFSCompletions []string       // cached filesystem completions
	lastCompletionInput string         // input that produced the cache
	// Tool selection collapse
	toolExpanded   bool // false = show collapsed single line when hasDefaultTool
	hasDefaultTool bool // true if default_tool is set in config
	// Options collapse
	optionsExpanded bool // false = show collapsed summary line
	// Worktree support. A single session name drives the branch AND the worktree
	// folder (both = the sanitized name, identical); there is no separate branch
	// field. The base is always the main repo root, so worktrees never nest.
	worktreeEnabled bool
	wtBaseRoot      string // main-repo root for the current path (de-nested; "" if not a git repo)
	wtLocation      string // [worktree].default_location (sibling / subdirectory / custom)
	wtTemplate      string // [worktree].path_template ("" = built-in sibling/subdirectory)
	// Inline validation error displayed inside the dialog
	validationErr string
}

// buildPresetCommands returns the list of commands for the picker,
// including any custom tools from config.toml.
func buildPresetCommands() []string {
	presets := []string{"", "claude", "gemini", "opencode", "codex", "copilot"}
	if customTools := session.GetCustomToolNames(); len(customTools) > 0 {
		presets = append(presets, customTools...)
	}
	return presets
}

// NewNewDialog creates a new NewDialog instance
func NewNewDialog() *NewDialog {
	// Create name input
	nameInput := textinput.New()
	nameInput.Placeholder = "session-name"
	nameInput.Focus()
	nameInput.CharLimit = MaxNameLength
	nameInput.Width = 40

	// Create path input
	pathInput := textinput.New()
	pathInput.Placeholder = "~/project/path"
	pathInput.CharLimit = 256
	pathInput.Width = 40

	// Get current working directory for default path
	cwd, err := os.Getwd()
	if err == nil {
		pathInput.SetValue(cwd)
	}

	// Create command input
	commandInput := textinput.New()
	commandInput.Placeholder = "custom command"
	commandInput.CharLimit = 100
	commandInput.Width = 40

	dlg := &NewDialog{
		nameInput:       nameInput,
		pathInput:       pathInput,
		commandInput:    commandInput,
		claudeOptions:   NewClaudeOptionsPanel(),
		geminiOptions:   NewYoloOptionsPanel("Gemini", "YOLO mode - auto-approve all"),
		codexOptions:    NewYoloOptionsPanel("Codex", "YOLO mode - bypass approvals and sandbox"),
		focusIndex:      0,
		visible:         false,
		presetCommands:  buildPresetCommands(),
		commandCursor:   0,
		parentGroupPath: "default",
		parentGroupName: "default",
		dropdownCursor:  -1,
		worktreeEnabled: false,
	}
	dlg.updateToolOptions()
	return dlg
}

// ShowInGroup shows the dialog with a pre-selected parent group and optional default path
func (d *NewDialog) ShowInGroup(groupPath, groupName, defaultPath string) {
	if groupPath == "" {
		groupPath = "default"
		groupName = "default"
	}
	d.parentGroupPath = groupPath
	d.parentGroupName = groupName
	d.visible = true
	d.focusIndex = 0
	d.validationErr = ""
	d.nameInput.SetValue("")
	d.nameInput.Focus()
	d.dropdownCursor = -1
	d.lastCompletionInput = ""
	d.cachedFSCompletions = nil
	d.pathInput.Blur()
	d.claudeOptions.Blur()
	d.geminiOptions.Blur()
	d.codexOptions.Blur()
	// Collapse tool/options by default
	d.toolExpanded = !d.hasDefaultTool
	d.optionsExpanded = false
	// Keep commandCursor at previously set default (don't reset to 0)
	d.updateToolOptions()
	// Reset worktree fields
	d.worktreeEnabled = false
	d.wtBaseRoot = ""
	// Set path input to group's default path if provided, otherwise use current working directory
	if defaultPath != "" {
		d.pathInput.SetValue(defaultPath)
	} else {
		cwd, err := os.Getwd()
		if err == nil {
			d.pathInput.SetValue(cwd)
		}
	}
	d.computeDropdownItems()
	// Initialize tool options from global config
	d.geminiOptions.SetDefaults(false)
	d.codexOptions.SetDefaults(false)
	if userConfig, err := session.LoadUserConfig(); err == nil && userConfig != nil {
		d.geminiOptions.SetDefaults(userConfig.Gemini.YoloMode)
		d.codexOptions.SetDefaults(userConfig.Codex.YoloMode)
		d.claudeOptions.SetDefaults(userConfig)
	}
	// Worktree location/template for the resolved-path preview.
	wt := session.GetWorktreeSettings()
	d.wtLocation = wt.DefaultLocation
	d.wtTemplate = wt.Template()
}

// SetDefaultTool sets the pre-selected command based on tool name
// Call this before Show/ShowInGroup to apply user's preferred default
func (d *NewDialog) SetDefaultTool(tool string) {
	if tool == "" {
		d.commandCursor = 0 // Default to shell
		d.hasDefaultTool = false
		return
	}

	// Find the tool in preset commands
	for i, cmd := range d.presetCommands {
		if cmd == tool {
			d.commandCursor = i
			d.hasDefaultTool = true
			d.updateToolOptions()
			return
		}
	}

	// Tool not found in presets, default to shell
	d.commandCursor = 0
	d.hasDefaultTool = false
	d.updateToolOptions()
}

// GetSelectedGroup returns the parent group path
func (d *NewDialog) GetSelectedGroup() string {
	return d.parentGroupPath
}

// SetSize sets the dialog dimensions
func (d *NewDialog) SetSize(width, height int) {
	d.width = width
	d.height = height
}

// SetPathSuggestions sets the available path suggestions for autocomplete
func (d *NewDialog) SetPathSuggestions(paths []string) {
	d.pathSuggestions = paths
	d.dropdownCursor = -1
	d.computeDropdownItems()
}

// dropdownItem is one row in the path picker. path is the canonical value
// inserted into the input when accepted; isRecent marks known project paths
// (rendered with a ★) versus freshly-scanned filesystem directories.
type dropdownItem struct {
	path     string
	isRecent bool
}

// collapseHomePath rewrites a leading home directory as "~" for display.
func collapseHomePath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return "~" + p[len(home):]
	}
	return p
}

// splitPathDisplay returns the last path segment (the name to scan for) and
// its tilde-collapsed parent directory, for the name-forward row layout.
func splitPathDisplay(p string) (name, parent string) {
	c := strings.TrimRight(collapseHomePath(p), string(os.PathSeparator))
	if c == "" {
		return string(os.PathSeparator), ""
	}
	name = filepath.Base(c)
	parent = filepath.Dir(c)
	if parent == "." {
		parent = ""
	}
	return name, parent
}

// computeDropdownItems filters path suggestions and filesystem completions
// based on the current text input value.
func (d *NewDialog) computeDropdownItems() {
	input := d.pathInput.Value()
	var items []dropdownItem
	seen := make(map[string]bool)

	// Filter recent paths by case-insensitive substring
	if input == "" {
		for _, p := range d.pathSuggestions {
			items = append(items, dropdownItem{path: p, isRecent: true})
			seen[p] = true
		}
	} else {
		lower := strings.ToLower(input)
		for _, p := range d.pathSuggestions {
			if strings.Contains(strings.ToLower(p), lower) {
				items = append(items, dropdownItem{path: p, isRecent: true})
				seen[p] = true
			}
		}
	}

	// Filesystem completions for path-like input
	if strings.Contains(input, "/") || strings.HasPrefix(input, "~") {
		if input != d.lastCompletionInput {
			d.lastCompletionInput = input
			if matches, err := session.GetDirectoryCompletions(input); err == nil {
				d.cachedFSCompletions = matches
			} else {
				d.cachedFSCompletions = nil
			}
		}
		// Deduplicate against recent path matches
		for _, m := range d.cachedFSCompletions {
			if !seen[m] {
				items = append(items, dropdownItem{path: m, isRecent: false})
			}
		}
	} else {
		d.cachedFSCompletions = nil
		d.lastCompletionInput = ""
	}

	d.dropdownItems = items
	if d.dropdownCursor >= len(d.dropdownItems) {
		d.dropdownCursor = len(d.dropdownItems) - 1
	}
}

// Show makes the dialog visible (uses default group)
func (d *NewDialog) Show() {
	d.ShowInGroup("default", "default", "")
}

// Hide hides the dialog
func (d *NewDialog) Hide() {
	d.visible = false
}

// IsVisible returns whether the dialog is visible
func (d *NewDialog) IsVisible() bool {
	return d.visible
}

// GetValues returns the current dialog values with expanded paths
func (d *NewDialog) GetValues() (name, path, command string) {
	name = strings.TrimSpace(d.nameInput.Value())
	// Fix: sanitize input to remove surrounding quotes that cause path issues
	path = strings.Trim(strings.TrimSpace(d.pathInput.Value()), "'\"")

	// Fix malformed paths that have ~ in the middle (e.g., "/some/path~/actual/path")
	// This can happen when textinput suggestion appends instead of replaces
	if idx := strings.Index(path, "~/"); idx > 0 {
		// Extract the part after the malformed prefix (the actual tilde-prefixed path)
		path = path[idx:]
	}

	// Expand tilde in path (handles both "~/" prefix and just "~")
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	} else if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			path = home
		}
	}

	// Get command - either from preset or custom input
	if d.commandCursor < len(d.presetCommands) {
		command = d.presetCommands[d.commandCursor]
	}
	if command == "" && d.commandInput.Value() != "" {
		command = strings.TrimSpace(d.commandInput.Value())
	}

	return name, path, command
}

// ToggleWorktree toggles the worktree checkbox. When enabling, it resolves the
// base repo root for the current path (de-nested to the main worktree) so the
// resolved-path preview is accurate.
func (d *NewDialog) ToggleWorktree() {
	d.worktreeEnabled = !d.worktreeEnabled
	if d.worktreeEnabled {
		d.refreshWorktreeBaseRoot()
	}
}

// refreshWorktreeBaseRoot resolves the main-repo root for the current path.
// GetWorktreeBaseRoot maps a worktree path back to its primary worktree, so a
// new worktree is always based off the main repo — never nested inside another
// worktree. Left empty when the path isn't a git repo (preview says so).
func (d *NewDialog) refreshWorktreeBaseRoot() {
	path := strings.Trim(strings.TrimSpace(d.pathInput.Value()), "'\"")
	if path == "" {
		d.wtBaseRoot = ""
		return
	}
	if root, err := git.GetWorktreeBaseRoot(path); err == nil {
		d.wtBaseRoot = root
	} else {
		d.wtBaseRoot = ""
	}
}

// derivedBranch is the single source of truth for both the branch name and the
// worktree folder name: the sanitized, lowercased session name (no prefix — all
// three identical). Lowercasing here flows to the folder too, since the folder
// name is derived from the branch.
func (d *NewDialog) derivedBranch() string {
	return strings.ToLower(git.SanitizeBranchName(strings.TrimSpace(d.nameInput.Value())))
}

// IsWorktreeEnabled returns whether worktree mode is enabled
func (d *NewDialog) IsWorktreeEnabled() bool {
	return d.worktreeEnabled
}

// GetValuesWithWorktree returns all values including worktree settings. The
// branch is always the sanitized session name (which also becomes the worktree
// folder name — all three identical).
func (d *NewDialog) GetValuesWithWorktree() (name, path, command, branch string, worktreeEnabled bool) {
	name, path, command = d.GetValues()
	if d.worktreeEnabled {
		branch = d.derivedBranch()
	}
	worktreeEnabled = d.worktreeEnabled
	return
}

// IsGeminiYoloMode returns whether YOLO mode is enabled for Gemini
func (d *NewDialog) IsGeminiYoloMode() bool {
	return d.geminiOptions.GetYoloMode()
}

// GetCodexYoloMode returns the Codex YOLO mode state
func (d *NewDialog) GetCodexYoloMode() bool {
	return d.codexOptions.GetYoloMode()
}

// GetSelectedCommand returns the currently selected command/tool
func (d *NewDialog) GetSelectedCommand() string {
	if d.commandCursor >= 0 && d.commandCursor < len(d.presetCommands) {
		return d.presetCommands[d.commandCursor]
	}
	return ""
}

// GetClaudeOptions returns the Claude-specific options (only relevant if command is "claude")
func (d *NewDialog) GetClaudeOptions() *session.ClaudeOptions {
	if !d.isClaudeSelected() {
		return nil
	}
	return d.claudeOptions.GetOptions()
}

// isClaudeSelected returns true if "claude" is the selected command
func (d *NewDialog) isClaudeSelected() bool {
	return d.commandCursor < len(d.presetCommands) && d.presetCommands[d.commandCursor] == "claude"
}

// Validate checks if the dialog values are valid and returns an error message if not
func (d *NewDialog) Validate() string {
	name := strings.TrimSpace(d.nameInput.Value())
	// Fix: sanitize input to remove surrounding quotes that cause path issues
	path := strings.Trim(strings.TrimSpace(d.pathInput.Value()), "'\"")

	// Check for empty name
	if name == "" {
		return "Session name cannot be empty"
	}

	// Check name length
	if len(name) > MaxNameLength {
		return fmt.Sprintf("Session name too long (max %d characters)", MaxNameLength)
	}

	// Check for empty path
	if path == "" {
		return "Project path cannot be empty"
	}

	// Validate the branch derived from the session name if worktree is enabled.
	if d.worktreeEnabled {
		branch := d.derivedBranch()
		if branch == "" {
			return "Session name needs letters or numbers to make a worktree branch"
		}
		if err := git.ValidateBranchName(branch); err != nil {
			return err.Error()
		}
	}

	return "" // Valid
}

// SetError sets an inline validation error displayed inside the dialog
func (d *NewDialog) SetError(msg string) {
	d.validationErr = msg
}

// ClearError clears the inline validation error
func (d *NewDialog) ClearError() {
	d.validationErr = ""
}

// optionsStartIndex returns the focus index where tool options begin.
// Worktree adds no focus slot (it's a checkbox + derived preview, no input).
func (d *NewDialog) optionsStartIndex() int {
	return 3 // 0=name, 1=path, 2=command, 3=options
}

// updateToolOptions sets d.toolOptions to the panel matching the current tool selection.
func (d *NewDialog) updateToolOptions() {
	switch d.GetSelectedCommand() {
	case "claude":
		d.toolOptions = d.claudeOptions
	case "gemini":
		d.toolOptions = d.geminiOptions
	case "codex":
		d.toolOptions = d.codexOptions
	default:
		d.toolOptions = nil
	}
}

func (d *NewDialog) updateFocus() {
	d.nameInput.Blur()
	d.pathInput.Blur()
	d.commandInput.Blur()
	d.claudeOptions.Blur()
	d.geminiOptions.Blur()
	d.codexOptions.Blur()

	switch d.focusIndex {
	case 0:
		d.nameInput.Focus()
	case 1:
		d.pathInput.Focus()
	case 2:
		if d.commandCursor == 0 { // shell
			d.commandInput.Focus()
		}
	default: // 3+ = tool options
		if d.toolOptions != nil {
			d.toolOptions.Focus()
		}
	}
}

// getMaxFocusIndex returns the maximum focus index based on current state
func (d *NewDialog) getMaxFocusIndex() int {
	max := 2 // 0=name, 1=path, 2=command/tool (reachable even when collapsed)
	if d.toolOptions != nil && d.optionsExpanded {
		max++ // +options (worktree adds no focus slot)
	}
	return max
}

// nextFocusIndex returns the next valid focus index after current, skipping collapsed fields.
func (d *NewDialog) nextFocusIndex(current int) int {
	maxIdx := d.getMaxFocusIndex()
	next := current + 1
	// Index 2 (tool) is always reachable, even when collapsed: it acts as a
	// non-text summary line from which t/w/a work. Skipping it used to trap
	// users with a default_tool (only text fields reachable, so those keys typed).
	if next > maxIdx {
		next = 0
	}
	return next
}

// prevFocusIndex returns the previous valid focus index before current, skipping collapsed fields.
func (d *NewDialog) prevFocusIndex(current int) int {
	maxIdx := d.getMaxFocusIndex()
	prev := current - 1
	// Index 2 (tool) is always reachable, even when collapsed (see nextFocusIndex).
	if prev < 0 {
		prev = maxIdx
	}
	return prev
}

func (d *NewDialog) isTextInputFocused() bool {
	switch d.focusIndex {
	case 0: // name input
		return true
	case 1: // path input
		return true
	case 2: // command — only when shell/custom is selected and expanded
		return d.commandCursor == 0 && d.toolExpanded
	}
	return false
}

// Update handles key messages
func (d *NewDialog) Update(msg tea.Msg) (*NewDialog, tea.Cmd) {
	if !d.visible {
		return d, nil
	}

	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			// Path field: accept dropdown selection if highlighted
			if d.focusIndex == 1 && d.dropdownCursor >= 0 && d.dropdownCursor < len(d.dropdownItems) {
				d.pathInput.SetValue(d.dropdownItems[d.dropdownCursor].path)
				d.pathInput.SetCursor(len(d.pathInput.Value()))
				if d.worktreeEnabled {
					d.refreshWorktreeBaseRoot()
				}
			}
			// Move to next field
			d.focusIndex = d.nextFocusIndex(d.focusIndex)
			if d.toolOptions != nil && d.optionsExpanded && d.focusIndex >= d.optionsStartIndex() {
				// Already in options panel — let it handle internal tab
				return d, d.toolOptions.Update(msg)
			}
			d.updateFocus()
			return d, cmd

		case "down":
			// Path field: navigate dropdown
			if d.focusIndex == 1 && len(d.dropdownItems) > 0 {
				if d.dropdownCursor < len(d.dropdownItems)-1 {
					d.dropdownCursor++
				}
				return d, nil
			}
			if d.focusIndex < d.getMaxFocusIndex() {
				d.focusIndex = d.nextFocusIndex(d.focusIndex)
				d.updateFocus()
			} else if d.toolOptions != nil && d.optionsExpanded && d.focusIndex >= d.optionsStartIndex() {
				return d, d.toolOptions.Update(msg)
			}
			return d, nil

		case "up":
			// Path field: navigate dropdown (can reach -1 = no selection)
			if d.focusIndex == 1 && d.dropdownCursor > -1 {
				d.dropdownCursor--
				return d, nil
			}
			fallthrough

		case "shift+tab":
			if d.toolOptions != nil && d.optionsExpanded && d.focusIndex >= d.optionsStartIndex() && !d.toolOptions.AtTop() {
				return d, d.toolOptions.Update(msg)
			}
			d.focusIndex = d.prevFocusIndex(d.focusIndex)
			d.updateFocus()
			return d, nil

		case "esc":
			// Path field: clear filter to show all recent paths
			if d.focusIndex == 1 && d.pathInput.Value() != "" {
				d.pathInput.SetValue("")
				d.dropdownCursor = -1
				d.computeDropdownItems()
				return d, nil
			}
			// Tool expanded: collapse it
			if d.focusIndex == 2 && d.toolExpanded && d.hasDefaultTool {
				d.toolExpanded = false
				d.focusIndex = d.nextFocusIndex(1) // skip past collapsed command
				d.updateFocus()
				return d, nil
			}
			// Options expanded: collapse them
			if d.optionsExpanded && d.focusIndex >= d.optionsStartIndex() {
				d.optionsExpanded = false
				// Move focus back to the tool field
				d.focusIndex = 2
				d.updateFocus()
				return d, nil
			}
			d.Hide()
			return d, nil

		case "enter":
			// Path field: accept dropdown selection if highlighted, advance focus
			if d.focusIndex == 1 {
				if d.dropdownCursor >= 0 && d.dropdownCursor < len(d.dropdownItems) {
					d.pathInput.SetValue(d.dropdownItems[d.dropdownCursor].path)
					d.pathInput.SetCursor(len(d.pathInput.Value()))
					if d.worktreeEnabled {
						d.refreshWorktreeBaseRoot()
					}
				}
				d.focusIndex = d.nextFocusIndex(d.focusIndex)
				d.updateFocus()
				return d, nil
			}
			// Let parent handle enter (create session)
			return d, nil

		case "left":
			if d.focusIndex == 2 && d.toolExpanded {
				d.commandCursor--
				if d.commandCursor < 0 {
					d.commandCursor = len(d.presetCommands) - 1
				}
				d.updateToolOptions()
				d.updateFocus()
				return d, nil
			}
			if d.toolOptions != nil && d.optionsExpanded && d.focusIndex >= d.optionsStartIndex() {
				return d, d.toolOptions.Update(msg)
			}

		case "right":
			if d.focusIndex == 2 && d.toolExpanded {
				d.commandCursor = (d.commandCursor + 1) % len(d.presetCommands)
				d.updateToolOptions()
				d.updateFocus()
				return d, nil
			}
			if d.toolOptions != nil && d.optionsExpanded && d.focusIndex >= d.optionsStartIndex() {
				return d, d.toolOptions.Update(msg)
			}

		case "t":
			// Expand tool picker — only when not in a text input field
			if !d.isTextInputFocused() && d.hasDefaultTool && !d.toolExpanded {
				d.toolExpanded = true
				d.focusIndex = 2
				d.updateFocus()
				return d, nil
			}

		case "a":
			// Toggle advanced options — only when not in a text input field
			if !d.isTextInputFocused() && d.toolOptions != nil {
				d.optionsExpanded = !d.optionsExpanded
				if d.optionsExpanded {
					d.focusIndex = d.optionsStartIndex()
					d.updateFocus()
				}
				return d, nil
			}

		case "w":
			// Toggle worktree when on the tool field (focusIndex == 2), whether
			// the tool picker is expanded or collapsed. Safe to fire here because
			// a collapsed tool field is not a focused text input. Worktree has no
			// input field (the branch/folder derive from the name), so focus stays.
			if d.focusIndex == 2 {
				d.ToggleWorktree()
				return d, nil
			}

		case "y":
			// 'y' shortcut from command field (gemini/codex only)
			selectedCmd := d.GetSelectedCommand()
			if d.focusIndex == 2 && d.toolExpanded && (selectedCmd == "gemini" || selectedCmd == "codex") && d.toolOptions != nil {
				d.toolOptions.Update(msg)
				return d, nil
			}
			if d.toolOptions != nil && d.optionsExpanded && d.focusIndex >= d.optionsStartIndex() {
				d.toolOptions.Update(msg)
				return d, nil
			}

		case " ":
			if d.toolOptions != nil && d.optionsExpanded && d.focusIndex >= d.optionsStartIndex() {
				return d, d.toolOptions.Update(msg)
			}

		default:
		}
	}

	// Update focused input
	switch d.focusIndex {
	case 0:
		// Name drives the branch + worktree folder; the preview reads it live.
		d.nameInput, cmd = d.nameInput.Update(msg)
	case 1:
		oldValue := d.pathInput.Value()
		d.pathInput, cmd = d.pathInput.Update(msg)
		if d.pathInput.Value() != oldValue {
			d.dropdownCursor = -1
			d.computeDropdownItems()
			// Keep the worktree base root (and its preview) in sync with the path.
			if d.worktreeEnabled {
				d.refreshWorktreeBaseRoot()
			}
		}
	case 2:
		if d.commandCursor == 0 && d.toolExpanded {
			d.commandInput, cmd = d.commandInput.Update(msg)
		}
	default: // 3+ = tool options
		if d.toolOptions != nil && d.optionsExpanded && d.focusIndex >= d.optionsStartIndex() {
			cmd = d.toolOptions.Update(msg)
		}
	}

	return d, cmd
}

// View renders the dialog
func (d *NewDialog) View() string {
	if !d.visible {
		return ""
	}

	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorCyan).
		MarginBottom(1)

	labelStyle := lipgloss.NewStyle().
		Foreground(ColorText)

	dimStyle := lipgloss.NewStyle().
		Foreground(ColorComment)

	// Responsive dialog width
	dialogWidth := 60
	if d.width > 0 && d.width < dialogWidth+10 {
		dialogWidth = d.width - 10
		if dialogWidth < 40 {
			dialogWidth = 40
		}
	}

	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorCyan).
		Background(ColorSurface).
		Padding(2, 4).
		Width(dialogWidth)

	// Active field indicator style
	activeLabelStyle := lipgloss.NewStyle().
		Foreground(ColorCyan).
		Bold(true)

	// Build content
	var content strings.Builder

	// Title with parent group info
	content.WriteString(titleStyle.Render("New Session"))
	content.WriteString("\n")
	groupInfoStyle := lipgloss.NewStyle().Foreground(ColorPurple)
	content.WriteString(groupInfoStyle.Render("  in group: " + d.parentGroupName))
	content.WriteString("\n\n")

	// Name input
	if d.focusIndex == 0 {
		content.WriteString(activeLabelStyle.Render("▶ Name:"))
	} else {
		content.WriteString(labelStyle.Render("  Name:"))
	}
	content.WriteString("\n")
	content.WriteString("  ")
	content.WriteString(d.nameInput.View())
	content.WriteString("\n\n")

	// Path section
	if d.focusIndex == 1 {
		content.WriteString(activeLabelStyle.Render("▶ Path:"))
	} else {
		content.WriteString(labelStyle.Render("  Path:"))
	}
	content.WriteString("\n")

	// Always show text input
	content.WriteString("  ")
	content.WriteString(d.pathInput.View())
	content.WriteString("\n")

	// Show filtered dropdown when path field is focused and has items.
	// Name-forward layout: the last path segment (what you scan for) is shown
	// first and bright; its parent dir trails dim; ★ marks recent projects.
	if d.focusIndex == 1 && len(d.dropdownItems) > 0 {
		parentStyle := lipgloss.NewStyle().Foreground(ColorComment)
		nameStyle := lipgloss.NewStyle().Foreground(ColorText)
		selectedStyle := lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
		starStyle := lipgloss.NewStyle().Foreground(ColorYellow)
		moreStyle := parentStyle

		maxShow := 5
		total := len(d.dropdownItems)
		startIdx := 0
		endIdx := total
		if total > maxShow {
			// Center the window around the cursor (or start from 0 if no selection)
			center := d.dropdownCursor
			if center < 0 {
				center = 0
			}
			startIdx = center - maxShow/2
			if startIdx < 0 {
				startIdx = 0
			}
			endIdx = startIdx + maxShow
			if endIdx > total {
				endIdx = total
				startIdx = endIdx - maxShow
			}
		}

		// Size the name column to the widest visible name, clamped for sanity.
		const nameColMin, nameColMax = 12, 28
		nameCol := nameColMin
		for i := startIdx; i < endIdx; i++ {
			n, _ := splitPathDisplay(d.dropdownItems[i].path)
			if len(n) > nameCol {
				nameCol = len(n)
			}
		}
		if nameCol > nameColMax {
			nameCol = nameColMax
		}

		if startIdx > 0 {
			content.WriteString(moreStyle.Render(fmt.Sprintf("      ↑ %d more above", startIdx)))
			content.WriteString("\n")
		}

		for i := startIdx; i < endIdx; i++ {
			it := d.dropdownItems[i]
			name, parent := splitPathDisplay(it.path)
			if len(name) > nameCol {
				name = name[:nameCol-1] + "…"
			}

			// Column 1: selection caret. Column 2: recent star.
			cursor := " "
			if i == d.dropdownCursor {
				cursor = ">"
			}
			star := " "
			if it.isRecent {
				star = "★"
			}

			ns := nameStyle
			if i == d.dropdownCursor {
				ns = selectedStyle
			}
			paddedName := name + strings.Repeat(" ", nameCol-len([]rune(name)))

			content.WriteString("  ")
			content.WriteString(selectedStyle.Render(cursor))
			content.WriteString(" ")
			content.WriteString(starStyle.Render(star))
			content.WriteString(" ")
			content.WriteString(ns.Render(paddedName))
			if parent != "" {
				content.WriteString("  ")
				content.WriteString(parentStyle.Render(parent))
			}
			content.WriteString("\n")
		}

		if endIdx < total {
			content.WriteString(moreStyle.Render(fmt.Sprintf("      ↓ %d more below", total-endIdx)))
			content.WriteString("\n")
		}
	}
	content.WriteString("\n")

	// Command/Tool selection
	if d.hasDefaultTool && !d.toolExpanded {
		// Collapsed: show single line with default tool
		toolName := d.GetSelectedCommand()
		if toolName == "" {
			toolName = "shell"
		}
		selectedStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
		if d.focusIndex == 2 {
			content.WriteString(activeLabelStyle.Render("▶ Tool: "))
		} else {
			content.WriteString("  Tool: ")
		}
		content.WriteString(selectedStyle.Render(toolName))
		content.WriteString(" ")
		if d.focusIndex == 2 {
			hint := "(t change · w worktree · a options)"
			if d.worktreeEnabled {
				hint = "(t change · w worktree ✓ · a options)"
			}
			content.WriteString(dimStyle.Render(hint))
		} else {
			content.WriteString(dimStyle.Render("(t to change)"))
		}
		content.WriteString("\n\n")
	} else {
		// Expanded: full pill bar
		if d.focusIndex == 2 {
			content.WriteString(activeLabelStyle.Render("▶ Command:"))
		} else {
			content.WriteString(labelStyle.Render("  Command:"))
		}
		content.WriteString("\n  ")

		var cmdButtons []string
		for i, cmd := range d.presetCommands {
			displayName := cmd
			if displayName == "" {
				displayName = "shell"
			}
			if icon := session.GetToolIcon(cmd); cmd != "" && icon != "" {
				if toolDef := session.GetToolDef(cmd); toolDef != nil && toolDef.Icon != "" {
					displayName = icon + " " + displayName
				}
			}

			var btnStyle lipgloss.Style
			if i == d.commandCursor {
				btnStyle = lipgloss.NewStyle().
					Foreground(ColorBg).
					Background(ColorAccent).
					Bold(true).
					Padding(0, 2)
			} else {
				btnStyle = lipgloss.NewStyle().
					Foreground(ColorTextDim).
					Background(ColorSurface).
					Padding(0, 2)
			}

			cmdButtons = append(cmdButtons, btnStyle.Render(displayName))
		}
		content.WriteString(lipgloss.JoinHorizontal(lipgloss.Left, cmdButtons...))
		content.WriteString("\n\n")

		// Custom command input (only if shell is selected)
		if d.commandCursor == 0 {
			if d.focusIndex == 2 {
				content.WriteString(activeLabelStyle.Render("  ▸ Custom:"))
			} else {
				content.WriteString(labelStyle.Render("    Custom:"))
			}
			content.WriteString("\n    ")
			content.WriteString(d.commandInput.View())
			content.WriteString("\n\n")
		}

		// Worktree checkbox (show when tool is expanded)
		worktreeLabel := "Create in worktree"
		if d.focusIndex == 2 {
			worktreeLabel = "Create in worktree (press w)"
		}
		content.WriteString(renderCheckboxLine(worktreeLabel, d.worktreeEnabled, d.focusIndex == 2))
	}

	// Worktree preview (only when enabled): the resolved full path + branch,
	// both derived from the session name. No input field — the name is the
	// single source, and the path is de-nested to the main repo root.
	if d.worktreeEnabled {
		content.WriteString("\n")
		content.WriteString(labelStyle.Render("  Worktree:"))
		content.WriteString("\n  ")
		branch := d.derivedBranch()
		switch {
		case branch == "":
			content.WriteString(dimStyle.Render("(type a session name)"))
		case d.wtBaseRoot == "":
			content.WriteString(dimStyle.Render("(selected path is not a git repository)"))
		default:
			target := git.WorktreePath(git.WorktreePathOptions{
				Branch:   branch,
				Location: d.wtLocation,
				RepoDir:  d.wtBaseRoot,
				Template: d.wtTemplate,
			})
			content.WriteString(dimStyle.Render(collapseHomePath(target)))
			content.WriteString("\n  ")
			content.WriteString(dimStyle.Render("branch " + branch))
		}
		content.WriteString("\n")
	}

	// Tool options panel — collapsed or expanded
	if d.toolOptions != nil {
		if d.optionsExpanded {
			content.WriteString("\n")
			content.WriteString(d.toolOptions.View())
		} else {
			// Collapsed summary
			content.WriteString("\n")
			if d.isClaudeSelected() {
				content.WriteString("  Options: ")
				content.WriteString(dimStyle.Render(d.claudeOptions.SummaryView()))
			} else {
				toolName := d.GetSelectedCommand()
				content.WriteString("  " + toolName + " options: ")
				content.WriteString(dimStyle.Render("defaults"))
			}
			content.WriteString(" ")
			content.WriteString(dimStyle.Render("(a for advanced)"))
			content.WriteString("\n")
		}
	}

	// Inline validation error
	if d.validationErr != "" {
		errStyle := lipgloss.NewStyle().Foreground(ColorRed).Bold(true)
		content.WriteString("\n")
		content.WriteString(errStyle.Render("  ⚠ " + d.validationErr))
	}

	content.WriteString("\n")

	// Help text
	helpStyle := lipgloss.NewStyle().
		Foreground(ColorComment).
		MarginTop(1)
	helpText := "Tab next │ ↑↓ navigate │ Enter create │ Esc cancel"
	if d.focusIndex == 1 {
		helpText = "↑↓ select │ Enter/Tab accept │ Esc cancel"
	} else if d.focusIndex == 2 && !d.toolExpanded {
		helpText = "t expand │ w worktree │ a options │ Tab next │ Enter create │ Esc cancel"
	} else if d.focusIndex == 2 && d.toolExpanded {
		selectedCmd := d.GetSelectedCommand()
		if selectedCmd == "gemini" || selectedCmd == "codex" {
			helpText = "←→ command │ w worktree │ y yolo │ Tab next │ Enter create │ Esc cancel"
		} else {
			helpText = "←→ command │ w worktree │ Tab next │ Enter create │ Esc cancel"
		}
	} else if d.toolOptions != nil && d.optionsExpanded && d.focusIndex >= d.optionsStartIndex() {
		helpText = "Space/y toggle │ ↑↓ navigate │ Enter create │ Esc cancel"
	}
	content.WriteString(helpStyle.Render(helpText))

	// Wrap in dialog box
	dialog := dialogStyle.Render(content.String())

	return lipgloss.Place(
		d.width,
		d.height,
		lipgloss.Center,
		lipgloss.Center,
		dialog,
	)
}
