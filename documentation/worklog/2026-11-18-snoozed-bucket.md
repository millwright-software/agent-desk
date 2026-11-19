# 2026-11-18 — One Snoozed bucket at the bottom of the list (v1.0.30)

**Ask:** "Could we move all of the snoozed sessions to a single snoozed group
down below ... Just one big snoozed bucket. I'm just trying to clear up stuff
in my left nav." Then: "merge it and cut v1.0.30, count only live sessions".

**What existed:** since v1.0.26 each group ended in its own `▸ N snoozed`
fold. With many groups that is a row per group for sessions he is not looking
at.

## Design

- **Display only.** `GroupTree.flatten` collects every snoozed top-level
  session from every group (with the sub-sessions under it), leaves them out
  of their groups' rows, and emits one `ItemTypeSnoozeFold` row at level 0
  after the last group. Open, the sessions follow it, soonest wake first,
  each row keeping its own group's `Path`. `GroupPath` never changes, so a
  wake returns the session to its place. This is not the v1.0.21 global
  Snoozed group (which moved sessions and needed `SnoozeFromGroup` to send
  them back); `migrateLegacySnoozed` still handles that one.
- **Collapsed groups count.** The collection runs over every group before the
  per-group loop, so a snooze inside a collapsed group is in the bucket.
  Before, collapsing a group hid its snoozed sessions.
- **The fold's key** is `session.SnoozeFoldKey` ("\x00snoozed"), both the
  row's `Path` and its key in `SnoozeOpen` / `snooze_open` in the UI state.
  Not a valid group path. Per-group keys saved by earlier versions are
  ignored, so the bucket starts closed after the upgrade.
- **Left arrow** on the row or a session inside it closes the fold and never
  falls through to "collapse the parent group": these rows sit outside every
  group.
- **Preview.** With the cursor on the row, the right pane lists the snoozed
  sessions under the group each will wake into (`renderSnoozeFoldPreview`).
  This is where his "micro groups" thought landed: the nav stays flat.
- **Header counts.** `LiveSessionCountForGroup` leaves out what the fold
  holds; the group header uses it. `SessionCountForGroup` is unchanged and
  still feeds `agent-desk group list`.

## Not verified

No one has looked at it on screen. `TestSnoozeFold` covers the row order,
paths, counts, the collapsed-group case and waking; the drawing of the row
and the preview is untested. The fold row has no `1·` hotkey prefix, so its
triangle may sit two columns left of the first nine group headers.

## Release

`go test -race ./...` green, `gofmt -l` empty. `goreleaser release --clean`
succeeded end to end, including the Homebrew cask: `millwright-software/
homebrew-tap` exists now (it 404'd on 11/11).
