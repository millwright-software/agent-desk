package ui

import "testing"

// ⚠️ THE FIELD MUST BE SIZED FROM THE BOX, NOT FROM A CONSTANT. pathInput.Width
// was set to 40 at construction and never revisited, so a project path longer
// than 40 cells — a worktree path on the reporter's machine is 66 — scrolled
// horizontally inside a window narrower than the border drawn around it. It
// read as "wraps weird and cuts off on the right until I arrow all the way
// right", which sounds like a rendering bug and is a sizing one.
func TestSetSizeResizesTheInputs(t *testing.T) {
	for _, termWidth := range []int{60, 80, 120, 171, 200, 250} {
		d := NewNewDialog()
		before := d.pathInput.Width
		d.SetSize(termWidth, 40)

		wantDialog, wantInput := dialogWidthFor(termWidth)
		if d.pathInput.Width != wantInput {
			t.Errorf("term %d: pathInput.Width = %d, want %d (was %d at construction)",
				termWidth, d.pathInput.Width, wantInput, before)
		}
		if d.nameInput.Width != wantInput || d.commandInput.Width != wantInput {
			t.Errorf("term %d: the other inputs were not resized (name %d, command %d, want %d)",
				termWidth, d.nameInput.Width, d.commandInput.Width, wantInput)
		}
		// ⚠️ The field has to FIT the box. If the input is wider than the
		// dialog's interior, the text runs under the border instead of
		// scrolling, which is the same complaint from the other direction.
		if wantInput >= wantDialog {
			t.Errorf("term %d: input %d does not fit inside dialog %d", termWidth, wantInput, wantDialog)
		}
	}
}

// ⚠️ AND THE DIALOG HAD NO BRANCH THAT GREW. It started at 60 and only ever
// shrank, so a 250-column terminal got the same 60-column box as an 80-column
// one and long paths stayed unreadable.
func TestDialogGrowsOnWideTerminalsAndShrinksOnNarrow(t *testing.T) {
	narrow, _ := dialogWidthFor(50)
	medium, _ := dialogWidthFor(80)
	wide, _ := dialogWidthFor(200)

	if narrow > 50 {
		t.Errorf("dialog %d does not fit a 50-column terminal", narrow)
	}
	if wide <= medium {
		t.Errorf("dialog did not grow: 200 cols → %d, 80 cols → %d", wide, medium)
	}
	// It must not swallow the whole screen either.
	if huge, _ := dialogWidthFor(400); huge > 110 {
		t.Errorf("dialog grew to %d on a 400-column terminal; expected a cap", huge)
	}
	// A path of the length that caused the report must fit without scrolling
	// on an ordinary terminal.
	const reportedPathLen = 66
	if _, input := dialogWidthFor(171); input < reportedPathLen {
		t.Errorf("input is %d cells at 171 columns; the reported path is %d", input, reportedPathLen)
	}
}

// Never returns something unusable, whatever it is handed.
func TestDialogWidthIsAlwaysSane(t *testing.T) {
	for _, w := range []int{0, -1, 1, 10, 39, 40, 41, 1000} {
		dialog, input := dialogWidthFor(w)
		if dialog < 40 {
			t.Errorf("term %d: dialog %d is below the 40 floor", w, dialog)
		}
		if input < 20 {
			t.Errorf("term %d: input %d is below the 20 floor", w, input)
		}
		if input >= dialog {
			t.Errorf("term %d: input %d does not fit dialog %d", w, input, dialog)
		}
	}
}
