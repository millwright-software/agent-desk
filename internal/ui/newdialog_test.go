package ui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewNewDialog(t *testing.T) {
	d := NewNewDialog()

	if d == nil {
		t.Fatal("NewNewDialog returned nil")
	}
	if d.IsVisible() {
		t.Error("Dialog should not be visible by default")
	}
	if len(d.presetCommands) == 0 {
		t.Error("presetCommands should not be empty")
	}
}

func TestDialogVisibility(t *testing.T) {
	d := NewNewDialog()

	d.Show()
	if !d.IsVisible() {
		t.Error("Dialog should be visible after Show()")
	}

	d.Hide()
	if d.IsVisible() {
		t.Error("Dialog should not be visible after Hide()")
	}
}

func TestDialogSetSize(t *testing.T) {
	d := NewNewDialog()
	d.SetSize(100, 50)

	if d.width != 100 {
		t.Errorf("Width = %d, want 100", d.width)
	}
	if d.height != 50 {
		t.Errorf("Height = %d, want 50", d.height)
	}
}

func TestDialogPresetCommands(t *testing.T) {
	d := NewNewDialog()

	// Should have shell (empty), claude, gemini, opencode, codex, copilot
	expectedCommands := []string{"", "claude", "gemini", "opencode", "codex", "copilot"}

	if len(d.presetCommands) != len(expectedCommands) {
		t.Errorf("Expected %d preset commands, got %d", len(expectedCommands), len(d.presetCommands))
	}

	for i, cmd := range expectedCommands {
		if d.presetCommands[i] != cmd {
			t.Errorf("presetCommands[%d] = %s, want %s", i, d.presetCommands[i], cmd)
		}
	}
}

func TestDialogGetValues(t *testing.T) {
	d := NewNewDialog()
	d.nameInput.SetValue("my-session")
	d.pathInput.SetValue("/tmp/project")
	d.commandCursor = 1 // claude

	name, path, command := d.GetValues()

	if name != "my-session" {
		t.Errorf("name = %s, want my-session", name)
	}
	if path != "/tmp/project" {
		t.Errorf("path = %s, want /tmp/project", path)
	}
	if command != "claude" {
		t.Errorf("command = %s, want claude", command)
	}
}

func TestDialogExpandTilde(t *testing.T) {
	d := NewNewDialog()
	d.nameInput.SetValue("test")
	d.pathInput.SetValue("~/projects")

	_, path, _ := d.GetValues()

	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(path, home) {
		t.Errorf("path should expand ~ to home directory, got %s", path)
	}
}

func TestDialogView(t *testing.T) {
	d := NewNewDialog()

	// Not visible - should return empty
	view := d.View()
	if view != "" {
		t.Error("View should be empty when not visible")
	}

	// Visible - should return content
	d.SetSize(80, 24)
	d.Show()
	view = d.View()
	if view == "" {
		t.Error("View should not be empty when visible")
	}
	if !strings.Contains(view, "New Session") {
		t.Error("View should contain 'New Session' title")
	}
}

func TestNewDialog_SetPathSuggestions(t *testing.T) {
	d := NewNewDialog()

	paths := []string{
		"/Users/test/project1",
		"/Users/test/project2",
		"/Users/test/other",
	}

	d.SetPathSuggestions(paths)

	if len(d.pathSuggestions) != 3 {
		t.Errorf("expected 3 suggestions, got %d", len(d.pathSuggestions))
	}
}

func TestNewDialog_MalformedPathFix(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal tilde path",
			input:    "~/projects/myapp",
			expected: home + "/projects/myapp",
		},
		{
			name:     "malformed path with cwd prefix",
			input:    "/Users/someone/claude-deck~/projects/myapp",
			expected: home + "/projects/myapp",
		},
		{
			name:     "already expanded path",
			input:    "/Users/ashesh/projects/myapp",
			expected: "/Users/ashesh/projects/myapp",
		},
		{
			name:     "just tilde",
			input:    "~",
			expected: home,
		},
		{
			name:     "malformed path with different prefix",
			input:    "/some/random/path~/other/path",
			expected: home + "/other/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewNewDialog()
			d.pathInput.SetValue(tt.input)

			_, path, _ := d.GetValues()

			if path != tt.expected {
				t.Errorf("GetValues() path = %q, want %q", path, tt.expected)
			}
		})
	}
}

// ===== Path Dropdown Tests =====

func TestNewDialog_DropdownPopulated_WhenSuggestionsExist(t *testing.T) {
	d := NewNewDialog()
	d.SetPathSuggestions([]string{"/a", "/b"})
	d.ShowInGroup("default", "default", "")

	if len(d.dropdownItems) == 0 {
		t.Error("dropdownItems should be populated when suggestions exist")
	}
	if d.dropdownCursor != -1 {
		t.Errorf("dropdownCursor should start at -1, got %d", d.dropdownCursor)
	}
}

func TestNewDialog_DropdownEmpty_WhenNoSuggestions(t *testing.T) {
	d := NewNewDialog()
	d.ShowInGroup("default", "default", "")

	// dropdownItems may contain filesystem completions from cwd, but
	// with no suggestions and no path-like input, should be based on cwd
	if d.dropdownCursor != -1 {
		t.Errorf("dropdownCursor should start at -1, got %d", d.dropdownCursor)
	}
}

func TestNewDialog_Dropdown_UpDownNavigates(t *testing.T) {
	d := NewNewDialog()
	d.SetPathSuggestions([]string{"/a", "/b", "/c"})
	d.Show()
	d.focusIndex = 1
	d.pathInput.SetValue("")
	d.computeDropdownItems()

	if d.dropdownCursor != -1 {
		t.Fatalf("dropdownCursor should start at -1, got %d", d.dropdownCursor)
	}

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.dropdownCursor != 0 {
		t.Errorf("dropdownCursor = %d after Down, want 0", d.dropdownCursor)
	}

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.dropdownCursor != 1 {
		t.Errorf("dropdownCursor = %d after 2nd Down, want 1", d.dropdownCursor)
	}

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.dropdownCursor != 2 {
		t.Errorf("dropdownCursor = %d after 3rd Down, want 2", d.dropdownCursor)
	}

	// Should not go past the end
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.dropdownCursor != 2 {
		t.Errorf("dropdownCursor = %d after 4th Down, want 2 (clamped)", d.dropdownCursor)
	}

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.dropdownCursor != 1 {
		t.Errorf("dropdownCursor = %d after Up, want 1", d.dropdownCursor)
	}

	// Up past 0 should reach -1 (no selection)
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.dropdownCursor != -1 {
		t.Errorf("dropdownCursor = %d after Up past 0, want -1", d.dropdownCursor)
	}
}

func TestNewDialog_Dropdown_EnterAcceptsAndAdvances(t *testing.T) {
	d := NewNewDialog()
	d.SetPathSuggestions([]string{"/first", "/second"})
	d.Show()
	d.focusIndex = 1
	d.pathInput.SetValue("")
	d.computeDropdownItems()
	d.dropdownCursor = 1

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	_, path, _ := d.GetValues()
	if path != "/second" {
		t.Errorf("path = %q, want /second", path)
	}
	if d.focusIndex == 1 {
		t.Error("focus should advance past path field after Enter")
	}
}

func TestNewDialog_Dropdown_TabAcceptsAndAdvances(t *testing.T) {
	d := NewNewDialog()
	d.SetPathSuggestions([]string{"/first", "/second"})
	d.Show()
	d.focusIndex = 1
	d.pathInput.SetValue("")
	d.computeDropdownItems()
	d.dropdownCursor = 0

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyTab})

	_, path, _ := d.GetValues()
	if path != "/first" {
		t.Errorf("path = %q, want /first", path)
	}
}

func TestNewDialog_Dropdown_EnterWithNoSelection_UsesRawText(t *testing.T) {
	d := NewNewDialog()
	d.Show()
	d.focusIndex = 1
	d.dropdownCursor = -1

	customPath := "/Users/test/brand-new-project"
	d.pathInput.SetValue(customPath)

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	_, path, _ := d.GetValues()
	if path != customPath {
		t.Errorf("Enter with no selection should use raw text\nGot: %q\nWant: %q", path, customPath)
	}
	if d.focusIndex == 1 {
		t.Error("focus should advance past path field after Enter")
	}
}

func TestNewDialog_TabDoesNotOverwriteCustomPath(t *testing.T) {
	d := NewNewDialog()
	d.Show()

	d.focusIndex = 1
	d.dropdownCursor = -1
	d.updateFocus()

	customPath := "/Users/test/brand-new-project"
	d.pathInput.SetValue(customPath)

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyTab})

	_, path, _ := d.GetValues()
	if path != customPath {
		t.Errorf("Tab overwrote custom path!\nGot: %q\nWant: %q", path, customPath)
	}
}

func TestNewDialog_Dropdown_TypingFilters(t *testing.T) {
	d := NewNewDialog()
	d.SetPathSuggestions([]string{"/projects/alpha", "/projects/beta", "/code/gamma"})
	d.Show()
	d.focusIndex = 1
	d.pathInput.SetValue("")
	d.computeDropdownItems()

	if len(d.dropdownItems) != 3 {
		t.Fatalf("expected 3 dropdown items with empty input, got %d", len(d.dropdownItems))
	}

	// Simulate typing "alpha" — filter should narrow
	d.pathInput.SetValue("alpha")
	d.dropdownCursor = -1
	d.computeDropdownItems()

	if len(d.dropdownItems) != 1 {
		t.Errorf("expected 1 dropdown item matching 'alpha', got %d", len(d.dropdownItems))
	}
	if len(d.dropdownItems) > 0 && d.dropdownItems[0].path != "/projects/alpha" {
		t.Errorf("expected /projects/alpha, got %s", d.dropdownItems[0].path)
	}
}

// ===== Collapsed Tool Selection Tests =====

func TestNewDialog_ToolCollapsed_WhenDefaultToolSet(t *testing.T) {
	d := NewNewDialog()
	d.SetDefaultTool("claude")
	d.ShowInGroup("default", "default", "")

	if d.toolExpanded {
		t.Error("tool should be collapsed when default_tool is set")
	}
	if !d.hasDefaultTool {
		t.Error("hasDefaultTool should be true")
	}
}

func TestNewDialog_ToolExpanded_WhenNoDefaultTool(t *testing.T) {
	d := NewNewDialog()
	d.SetDefaultTool("")
	d.ShowInGroup("default", "default", "")

	if !d.toolExpanded {
		t.Error("tool should be expanded when no default_tool")
	}
}

func TestNewDialog_ToolExpand_TKey(t *testing.T) {
	d := NewNewDialog()
	d.SetDefaultTool("claude")
	d.Show()

	// Move focus to command field with non-shell cursor so isTextInputFocused() returns false
	d.focusIndex = 2
	d.commandCursor = 1 // claude (not shell)

	// 't' should expand the tool picker
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})

	if !d.toolExpanded {
		t.Error("'t' should expand the tool picker")
	}
	if d.focusIndex != 2 {
		t.Errorf("focus should move to command field (2), got %d", d.focusIndex)
	}
}

func TestNewDialog_ToolCollapse_EscFromCommandField(t *testing.T) {
	d := NewNewDialog()
	d.SetDefaultTool("claude")
	d.Show()
	d.toolExpanded = true
	d.focusIndex = 2

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if d.toolExpanded {
		t.Error("Esc on command field should collapse tool picker")
	}
}

func TestNewDialog_TabReachesCollapsedTool(t *testing.T) {
	d := NewNewDialog()
	d.SetDefaultTool("claude")
	d.Show()
	d.focusIndex = 1 // path

	// Tab from path now LANDS on the collapsed tool field (index 2) rather than
	// skipping it. Skipping it trapped users with a default_tool: only text fields
	// were reachable, so the t/w/a shortcuts got typed as characters instead.
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyTab})
	if d.focusIndex != 2 {
		t.Fatalf("Tab should land on the collapsed tool field (2), got %d", d.focusIndex)
	}

	// w toggles worktree from the collapsed tool field (no expand needed).
	d.nameInput.SetValue("my feature")
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	if !d.worktreeEnabled {
		t.Error("w on the collapsed tool field should toggle worktree on")
	}
}

func TestNewDialog_View_CollapsedTool(t *testing.T) {
	d := NewNewDialog()
	d.SetDefaultTool("claude")
	d.SetSize(80, 40)
	d.Show()

	view := d.View()

	if !strings.Contains(view, "Tool:") {
		t.Error("collapsed tool should show 'Tool:' label")
	}
	if !strings.Contains(view, "t to change") {
		t.Error("collapsed tool should show '(t to change)' hint")
	}
}

// ===== Collapsed Options Tests =====

func TestNewDialog_OptionsCollapsed_ByDefault(t *testing.T) {
	d := NewNewDialog()
	d.Show()

	if d.optionsExpanded {
		t.Error("options should be collapsed by default")
	}
}

func TestNewDialog_OptionsToggle_AKey(t *testing.T) {
	d := NewNewDialog()
	d.commandCursor = 1 // claude
	d.updateToolOptions()
	d.Show()

	// Move focus to command field with non-shell cursor so isTextInputFocused() returns false
	d.focusIndex = 2
	d.toolExpanded = true

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if !d.optionsExpanded {
		t.Error("'a' should expand options")
	}

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if d.optionsExpanded {
		t.Error("'a' again should collapse options")
	}
}

func TestNewDialog_View_CollapsedOptions_ShowsSummary(t *testing.T) {
	d := NewNewDialog()
	d.commandCursor = 1 // claude
	d.updateToolOptions()
	d.SetSize(80, 40)
	d.Show()

	view := d.View()

	if !strings.Contains(view, "a for advanced") {
		t.Error("collapsed options should show '(a for advanced)' hint")
	}
}

// ===== Claude Options SummaryView Tests =====

func TestClaudeOptionsPanel_SummaryView_Defaults(t *testing.T) {
	p := NewClaudeOptionsPanel()
	if p.SummaryView() != "defaults" {
		t.Errorf("SummaryView = %q, want 'defaults'", p.SummaryView())
	}
}

func TestClaudeOptionsPanel_SummaryView_SkipPerms(t *testing.T) {
	p := NewClaudeOptionsPanel()
	p.skipPermissions = true
	s := p.SummaryView()
	if !strings.Contains(s, "skip-perms") {
		t.Errorf("SummaryView = %q, want to contain 'skip-perms'", s)
	}
}

func TestClaudeOptionsPanel_SummaryView_Multiple(t *testing.T) {
	p := NewClaudeOptionsPanel()
	p.skipPermissions = true
	p.useChrome = true
	s := p.SummaryView()
	if s != "skip-perms, chrome" {
		t.Errorf("SummaryView = %q, want 'skip-perms, chrome'", s)
	}
}

func TestClaudeOptionsPanel_SummaryView_ContinueMode(t *testing.T) {
	p := NewClaudeOptionsPanel()
	p.sessionMode = 1 // continue
	s := p.SummaryView()
	if !strings.Contains(s, "continue") {
		t.Errorf("SummaryView = %q, want to contain 'continue'", s)
	}
}

// ===== Worktree Support Tests =====

func TestNewDialog_WorktreeToggle(t *testing.T) {
	dialog := NewNewDialog()
	if dialog.worktreeEnabled {
		t.Error("Worktree should be disabled by default")
	}
	dialog.ToggleWorktree()
	if !dialog.worktreeEnabled {
		t.Error("Worktree should be enabled after toggle")
	}
	dialog.ToggleWorktree()
	if dialog.worktreeEnabled {
		t.Error("Worktree should be disabled after second toggle")
	}
}

func TestNewDialog_IsWorktreeEnabled(t *testing.T) {
	dialog := NewNewDialog()
	if dialog.IsWorktreeEnabled() {
		t.Error("IsWorktreeEnabled should return false by default")
	}
	dialog.worktreeEnabled = true
	if !dialog.IsWorktreeEnabled() {
		t.Error("IsWorktreeEnabled should return true when enabled")
	}
}

func TestNewDialog_GetValuesWithWorktree(t *testing.T) {
	dialog := NewNewDialog()
	dialog.worktreeEnabled = true
	dialog.branchInput.SetValue("feature/test")
	dialog.nameInput.SetValue("test-session")
	dialog.pathInput.SetValue("/tmp/project")

	name, path, command, branch, enabled := dialog.GetValuesWithWorktree()

	if !enabled {
		t.Error("worktreeEnabled should be true")
	}
	if branch != "feature/test" {
		t.Errorf("Branch: got %q, want %q", branch, "feature/test")
	}
	if name != "test-session" {
		t.Errorf("Name: got %q, want %q", name, "test-session")
	}
	if path != "/tmp/project" {
		t.Errorf("Path: got %q, want %q", path, "/tmp/project")
	}
	_ = command
}

func TestNewDialog_GetValuesWithWorktree_Disabled(t *testing.T) {
	dialog := NewNewDialog()
	dialog.worktreeEnabled = false
	dialog.branchInput.SetValue("feature/test")

	_, _, _, branch, enabled := dialog.GetValuesWithWorktree()

	if enabled {
		t.Error("worktreeEnabled should be false")
	}
	if branch != "feature/test" {
		t.Errorf("Branch: got %q, want %q", branch, "feature/test")
	}
}

func TestNewDialog_Validate_WorktreeEnabled_EmptyBranch(t *testing.T) {
	dialog := NewNewDialog()
	dialog.nameInput.SetValue("test-session")
	dialog.pathInput.SetValue("/tmp/project")
	dialog.worktreeEnabled = true
	dialog.branchInput.SetValue("")

	err := dialog.Validate()
	if err == "" {
		t.Error("Validation should fail when worktree enabled but branch is empty")
	}
	if err != "Branch name required for worktree" {
		t.Errorf("Unexpected error message: %q", err)
	}
}

func TestNewDialog_Validate_WorktreeEnabled_InvalidBranch(t *testing.T) {
	dialog := NewNewDialog()
	dialog.nameInput.SetValue("test-session")
	dialog.pathInput.SetValue("/tmp/project")
	dialog.worktreeEnabled = true
	dialog.branchInput.SetValue("feature..test") // Invalid: contains ..

	err := dialog.Validate()
	if err == "" {
		t.Error("Validation should fail for invalid branch name")
	}
	if err != "branch name cannot contain '..'" {
		t.Errorf("Unexpected error message: %q", err)
	}
}

func TestNewDialog_Validate_WorktreeEnabled_ValidBranch(t *testing.T) {
	dialog := NewNewDialog()
	dialog.nameInput.SetValue("test-session")
	dialog.pathInput.SetValue("/tmp/project")
	dialog.worktreeEnabled = true
	dialog.branchInput.SetValue("feature/test-branch")

	err := dialog.Validate()
	if err != "" {
		t.Errorf("Validation should pass for valid branch, got: %q", err)
	}
}

func TestNewDialog_Validate_WorktreeDisabled_IgnoresBranch(t *testing.T) {
	dialog := NewNewDialog()
	dialog.nameInput.SetValue("test-session")
	dialog.pathInput.SetValue("/tmp/project")
	dialog.worktreeEnabled = false
	dialog.branchInput.SetValue("")

	err := dialog.Validate()
	if err != "" {
		t.Errorf("Validation should pass when worktree disabled, got: %q", err)
	}
}

func TestNewDialog_ShowInGroup_ResetsWorktree(t *testing.T) {
	dialog := NewNewDialog()
	dialog.worktreeEnabled = true
	dialog.branchInput.SetValue("feature/old-branch")

	dialog.ShowInGroup("projects", "Projects", "")

	if dialog.worktreeEnabled {
		t.Error("worktreeEnabled should be reset to false on ShowInGroup")
	}
	if dialog.branchInput.Value() != "" {
		t.Errorf("branchInput should be reset, got: %q", dialog.branchInput.Value())
	}
}

func TestNewDialog_ShowInGroup_SetsDefaultPath(t *testing.T) {
	dialog := NewNewDialog()

	dialog.ShowInGroup("projects", "Projects", "/test/default/path")

	if dialog.pathInput.Value() != "/test/default/path" {
		t.Errorf("pathInput should be set to default path, got: %q", dialog.pathInput.Value())
	}
}

func TestNewDialog_ShowInGroup_EmptyDefaultPath(t *testing.T) {
	dialog := NewNewDialog()

	dialog.ShowInGroup("projects", "Projects", "")

	value := dialog.pathInput.Value()
	if value == "" {
		t.Error("pathInput should not be empty when defaultPath is empty (should use cwd)")
	}
}

func TestNewDialog_BranchInputInitialized(t *testing.T) {
	dialog := NewNewDialog()

	if dialog.branchInput.Placeholder != "feature/branch-name" {
		t.Errorf("branchInput placeholder: got %q, want %q",
			dialog.branchInput.Placeholder, "feature/branch-name")
	}
}

func TestNewDialog_WorktreeToggle_ViaKeyPress(t *testing.T) {
	dialog := NewNewDialog()
	dialog.Show()
	dialog.toolExpanded = true // must be expanded to use 'w'
	dialog.focusIndex = 2      // Command field

	dialog, _ = dialog.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})

	if !dialog.worktreeEnabled {
		t.Error("Worktree should be enabled after pressing 'w' on command field")
	}

	if dialog.focusIndex != 3 {
		t.Errorf("Focus should move to branch field (3), got %d", dialog.focusIndex)
	}

	dialog.focusIndex = 2
	dialog, _ = dialog.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})

	if dialog.worktreeEnabled {
		t.Error("Worktree should be disabled after pressing 'w' again")
	}
}

func TestNewDialog_View_ShowsWorktreeCheckbox(t *testing.T) {
	dialog := NewNewDialog()
	dialog.SetSize(80, 40)
	dialog.Show()
	dialog.toolExpanded = true // worktree checkbox only visible when tool expanded
	dialog.focusIndex = 2

	view := dialog.View()

	if !strings.Contains(view, "Create in worktree") {
		t.Error("View should contain 'Create in worktree' checkbox")
	}

	if !strings.Contains(view, "press w") {
		t.Error("View should contain 'press w' hint when on command field")
	}
}

func TestNewDialog_View_ShowsBranchInputWhenEnabled(t *testing.T) {
	dialog := NewNewDialog()
	dialog.SetSize(80, 40)
	dialog.Show()
	dialog.worktreeEnabled = true

	view := dialog.View()

	if !strings.Contains(view, "Branch:") {
		t.Error("View should contain 'Branch:' label when worktree enabled")
	}

	if !strings.Contains(view, "[x]") {
		t.Error("View should show checked checkbox [x] when worktree enabled")
	}
}

func TestNewDialog_View_HidesBranchInputWhenDisabled(t *testing.T) {
	dialog := NewNewDialog()
	dialog.SetSize(80, 40)
	dialog.Show()
	dialog.toolExpanded = true // checkbox only shown when expanded
	dialog.worktreeEnabled = false

	view := dialog.View()

	if strings.Contains(view, "Branch:") {
		t.Error("View should NOT contain 'Branch:' label when worktree disabled")
	}

	if !strings.Contains(view, "[ ]") {
		t.Error("View should show unchecked checkbox [ ] when worktree disabled")
	}
}

// ===== CharLimit & Inline Error Tests =====

func TestNewDialog_CharLimitMatchesMaxNameLength(t *testing.T) {
	d := NewNewDialog()
	if d.nameInput.CharLimit != MaxNameLength {
		t.Errorf("nameInput.CharLimit = %d, want %d (MaxNameLength)", d.nameInput.CharLimit, MaxNameLength)
	}
}

func TestNewDialog_CharLimitTruncatesLongNames(t *testing.T) {
	d := NewNewDialog()
	d.pathInput.SetValue("/tmp/project")
	longName := strings.Repeat("a", MaxNameLength+10)
	d.nameInput.SetValue(longName)

	actual := d.nameInput.Value()
	if len(actual) > MaxNameLength {
		t.Errorf("nameInput should truncate to MaxNameLength (%d), but got length %d", MaxNameLength, len(actual))
	}

	err := d.Validate()
	if err != "" {
		t.Errorf("Validate() should pass after CharLimit truncation, got: %q", err)
	}
}

func TestNewDialog_Validate_NameAtMaxLength(t *testing.T) {
	d := NewNewDialog()
	d.pathInput.SetValue("/tmp/project")
	exactName := strings.Repeat("a", MaxNameLength)
	d.nameInput.SetValue(exactName)

	err := d.Validate()
	if err != "" {
		t.Errorf("Validate() should accept name at exactly MaxNameLength, got: %q", err)
	}
}

func TestNewDialog_SetError_ShowsInView(t *testing.T) {
	d := NewNewDialog()
	d.SetSize(80, 40)
	d.Show()

	d.SetError("Something went wrong")
	view := d.View()

	if !strings.Contains(view, "Something went wrong") {
		t.Error("View should display the inline error message")
	}
}

func TestNewDialog_ClearError_HidesFromView(t *testing.T) {
	d := NewNewDialog()
	d.SetSize(80, 40)
	d.Show()

	d.SetError("Something went wrong")
	d.ClearError()
	view := d.View()

	if strings.Contains(view, "Something went wrong") {
		t.Error("View should not display the error after ClearError()")
	}
}

func TestNewDialog_ShowInGroup_ClearsError(t *testing.T) {
	d := NewNewDialog()
	d.SetError("Previous error")
	d.ShowInGroup("group", "Group", "")

	if d.validationErr != "" {
		t.Error("ShowInGroup should clear validationErr")
	}
}

// ===== Worktree Branch Auto-Matching Tests =====

func TestNewDialog_ToggleWorktree_AutoPopulatesBranch(t *testing.T) {
	d := NewNewDialog()
	d.nameInput.SetValue("amber-falcon")

	d.ToggleWorktree()

	if !d.worktreeEnabled {
		t.Fatal("worktreeEnabled should be true after toggle")
	}
	if d.branchInput.Value() != "feature/amber-falcon" {
		t.Errorf("branch = %q, want %q", d.branchInput.Value(), "feature/amber-falcon")
	}
	if !d.branchAutoSet {
		t.Error("branchAutoSet should be true after auto-population")
	}
}

func TestNewDialog_AutoBranch_ConfigurablePrefix(t *testing.T) {
	d := NewNewDialog()

	// Custom prefix is applied and the session name is sanitized into a valid branch.
	d.branchPrefix = "wip/"
	d.nameInput.SetValue("Dark Mode Toggle")
	d.autoBranchFromName()
	got := d.branchInput.Value()
	if !strings.HasPrefix(got, "wip/") {
		t.Errorf("branch %q should start with configured prefix %q", got, "wip/")
	}
	if strings.ContainsAny(got, " ") {
		t.Errorf("branch %q should be sanitized (no spaces)", got)
	}

	// Empty prefix disables prefixing (just the sanitized name).
	d.branchPrefix = ""
	d.branchAutoSet = true
	d.nameInput.SetValue("hotfix")
	d.autoBranchFromName()
	if got := d.branchInput.Value(); got != "hotfix" {
		t.Errorf("empty prefix: branch = %q, want %q", got, "hotfix")
	}
}

func TestNewDialog_ToggleWorktree_EmptyName_NoBranch(t *testing.T) {
	d := NewNewDialog()

	d.ToggleWorktree()

	if d.branchInput.Value() != "" {
		t.Errorf("branch should be empty when name is empty, got %q", d.branchInput.Value())
	}
}

func TestNewDialog_ShowInGroup_ResetsBranchAutoSet(t *testing.T) {
	d := NewNewDialog()
	d.branchAutoSet = true

	d.ShowInGroup("projects", "Projects", "")

	if d.branchAutoSet {
		t.Error("branchAutoSet should be reset to false on ShowInGroup")
	}
}

// ===== Text Input Guard Regression Tests =====

func TestNewDialog_TKey_DoesNotExpandTool_WhenNameFocused(t *testing.T) {
	d := NewNewDialog()
	d.SetDefaultTool("claude")
	d.Show() // focusIndex=0 (name field)

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})

	if d.toolExpanded {
		t.Error("'t' should NOT expand tool picker when name field is focused")
	}
}

func TestNewDialog_AKey_DoesNotToggleOptions_WhenNameFocused(t *testing.T) {
	d := NewNewDialog()
	d.commandCursor = 1 // claude
	d.updateToolOptions()
	d.Show() // focusIndex=0 (name field)

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	if d.optionsExpanded {
		t.Error("'a' should NOT toggle options when name field is focused")
	}
}

func TestNewDialog_TKey_TypesIntoPathInput(t *testing.T) {
	d := NewNewDialog()
	d.SetPathSuggestions([]string{"/a", "/b"})
	d.Show()
	d.focusIndex = 1
	d.pathInput.SetValue("")
	d.pathInput.Focus()

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})

	if d.pathInput.Value() != "t" {
		t.Errorf("'t' should type into path input, got %q", d.pathInput.Value())
	}
}

func TestNewDialog_AKey_TypesIntoPathInput(t *testing.T) {
	d := NewNewDialog()
	d.SetPathSuggestions([]string{"/a", "/b"})
	d.Show()
	d.focusIndex = 1
	d.pathInput.SetValue("")
	d.pathInput.Focus()

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	if d.pathInput.Value() != "a" {
		t.Errorf("'a' should type into path input, got %q", d.pathInput.Value())
	}
}
