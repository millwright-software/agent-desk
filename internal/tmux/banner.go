//go:build !windows
// +build !windows

package tmux

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-runewidth"
)

// AttachBanner is the name card shown for a moment over a freshly attached
// session, so you know where you landed after Shift+Right / Shift+Left.
//
// ⚠️ It is a tmux popup (display-popup), NOT something drawn into the proxied
// byte stream. tmux composites the popup over the pane and repaints what was
// underneath when it closes. An overlay written by hand into a stream that is
// also scrolling would be torn by the session's own output within the second
// it is on screen, and there would be nothing to restore from afterwards.
type AttachBanner struct {
	Title    string // the session's title, in bold
	Subtitle string // group path (or similar), dimmed; may be empty
}

const (
	// BannerSubcommand is the hidden CLI verb that draws the card inside the
	// popup. The popup runs the agent-desk binary itself so the card text is
	// passed as environment (-e), never through a shell, whatever the title.
	BannerSubcommand = "attach-banner"

	BannerEnvTitle    = "AGENT_DESK_BANNER_TITLE"
	BannerEnvSubtitle = "AGENT_DESK_BANNER_SUBTITLE"
	BannerEnvMillis   = "AGENT_DESK_BANNER_MS"
	BannerEnvTarget   = "AGENT_DESK_BANNER_TARGET" // tmux session to re-send a swallowed key to

	// bannerDuration is how long the card stays up if nothing is typed. One
	// second is enough now that nothing dismisses it early; two felt like a
	// wait.
	bannerDuration = time.Second

	// Outer popup sizes. The block-letter card is border, blank, five rows of
	// letters, blank, subtitle, blank, border. When the title is too wide for
	// the block font at this terminal size the card falls back to one bold
	// line: border, blank, title, subtitle, border. bannerMinWidth keeps a
	// one-letter title from rendering as a postage stamp.
	bannerArtHeight   = 2 + 1 + bannerFontRows + 1 + 1 + 1
	bannerPlainHeight = 5
	bannerMinWidth    = 24
	bannerPadding     = 8 // 2 border cells + 3 cells of air each side

	// bannerClientWait bounds how long we poll for the tmux client to appear.
	// The popup needs a client to draw on; the attach we just started takes a
	// few milliseconds to register with the server.
	bannerClientWait = 500 * time.Millisecond
	bannerClientPoll = 10 * time.Millisecond
)

var bannerLog = respawnLog

// bannerSize is the popup's outer width and height for this card on a client
// cols wide. The block font is used when it fits; otherwise the plain card.
//
// ⚠️ RenderBanner, running inside the popup, makes the same art-or-plain
// choice from the interior size it is given. The two agree by construction:
// the art card is only chosen here when the letters fit its interior, and the
// plain card's interior is too short for art, so the renderer cannot pick art
// in a plain-sized popup or vice versa.
func bannerSize(b AttachBanner, cols int) (width, height int) {
	maxWidth := cols - 4
	if cols <= 0 {
		maxWidth = 1 << 20 // unknown terminal size: do not clamp
	}
	if w := blockTextWidth(b.Title) + bannerPadding; blockTextFits(b.Title, maxWidth-bannerPadding) {
		width, height = w, bannerArtHeight
	} else {
		width, height = runewidth.StringWidth(b.Title)+bannerPadding, bannerPlainHeight
	}
	if sw := runewidth.StringWidth(b.Subtitle) + bannerPadding; sw > width {
		width = sw
	}
	if width < bannerMinWidth {
		width = bannerMinWidth
	}
	if width > maxWidth {
		width = maxWidth
	}
	if width < 10 {
		width = 10
	}
	return width, height
}

// bannerPopupArgs builds the display-popup invocation. Two trailing arguments
// (exe and the verb) make tmux exec the binary directly instead of through
// `sh -c`, so nothing here needs quoting.
func bannerPopupArgs(client, sessionName, exe string, b AttachBanner, cols int) []string {
	width, height := bannerSize(b, cols)
	return []string{
		"display-popup",
		"-E",            // close when the card's process exits
		"-b", "rounded", // border-lines
		"-c", client,
		"-t", sessionName, // target-pane: anchors the popup to the session, not to whatever tmux guesses is "current"
		"-w", strconv.Itoa(width),
		"-h", strconv.Itoa(height),
		"-e", BannerEnvTitle + "=" + b.Title,
		"-e", BannerEnvSubtitle + "=" + b.Subtitle,
		"-e", BannerEnvMillis + "=" + strconv.Itoa(int(bannerDuration/time.Millisecond)),
		"-e", BannerEnvTarget + "=" + sessionName,
		exe, BannerSubcommand,
	}
}

// RenderBanner lays the card out for a popup interior of cols x rows cells.
// With room for it, the title is drawn in the block font: a blank line, five
// rows of letters, a blank line, the subtitle dimmed. Otherwise it is a blank
// line, the title in bold, the subtitle dimmed. Every line is centred and
// truncated to fit. Lines are joined with CRLF so it renders the same whether
// or not the popup's pty translates newlines.
func RenderBanner(title, subtitle string, cols, rows int) string {
	if cols < 1 {
		cols = 1
	}
	centre := func(s string) string {
		s = runewidth.Truncate(s, cols, "…")
		pad := (cols - runewidth.StringWidth(s)) / 2
		if pad < 0 {
			pad = 0
		}
		return strings.Repeat(" ", pad) + s
	}

	var lines []string
	if rows >= bannerArtHeight-2 && blockTextFits(title, cols) {
		lines = append(lines, "")
		for _, row := range renderBlockText(title) {
			lines = append(lines, "\x1b[1m"+centre(row)+"\x1b[0m")
		}
		lines = append(lines, "")
	} else {
		lines = append(lines, "", "\x1b[1m"+centre(title)+"\x1b[0m")
	}
	if subtitle != "" {
		lines = append(lines, "\x1b[2m"+centre(subtitle)+"\x1b[0m")
	}
	if rows > 0 && len(lines) > rows {
		// Too short for the leading blank line: drop it, then whatever else
		// does not fit from the bottom.
		lines = lines[1:]
		if len(lines) > rows {
			lines = lines[:rows]
		}
	}
	return strings.Join(lines, "\r\n")
}

// bannerPopup is one card's lifetime: opened on the attach's tmux client once
// that client exists, dismissed early by the first keystroke.
type bannerPopup struct {
	sessionName string
	clientPID   int
	exe         string
	banner      AttachBanner
	cols        int
	started     time.Time // when the attach began, for the timing in the log

	mu     sync.Mutex
	client string // set just before the popup is opened
	done   bool   // dismissed or never shown; show() and dismiss() are both no-ops after this
}

// show waits for the attach client to register, then opens the popup. Run it
// in a goroutine: the attach loop must not wait on tmux for this.
//
// ⚠️ `display-popup -E` BLOCKS until the popup closes (measured: the full
// second). The mutex is therefore released before the command runs, or
// dismiss() would queue behind it and the first keystroke would stall for as
// long as the card is up.
func (p *bannerPopup) show(ctx context.Context) {
	client := waitForClient(ctx, p.sessionName, p.clientPID)
	if client == "" {
		return
	}
	p.mu.Lock()
	if p.done {
		p.mu.Unlock()
		return // a key arrived before we got here; the user is already typing
	}
	p.client = client
	p.mu.Unlock()

	args := bannerPopupArgs(client, p.sessionName, p.exe, p.banner, p.cols)
	opened := time.Now()
	bannerLog.Debug("attach_banner_open", slog.String("client", client), slog.Duration("after_attach", time.Since(p.started)))
	out, err := exec.Command("tmux", args...).CombinedOutput()
	// Exit 129 is tmux's code for "closed by display-popup -C", i.e. our own
	// dismiss(); 0 is the card timing out (or exiting on a key it got
	// directly). Anything else is old tmux (pre-3.3 lacks -b) or a client
	// that went away; none of it is worth interrupting the attach over.
	bannerLog.Debug("attach_banner_closed",
		slog.Duration("shown_for", time.Since(opened)),
		slog.Any("error", err),
		slog.String("output", strings.TrimSpace(string(out))))
}

// dismiss closes the popup so a keystroke reaches the session instead of the
// card. ⚠️ Call it BEFORE forwarding the key: tmux processes the close (a
// separate command client that returns synchronously) before it reads the
// bytes we write to the attach pty afterwards, so ordering is deterministic.
//
// There is a few-millisecond window where show() has committed to opening
// the popup but tmux has not drawn it yet; a -C then is a no-op and the card
// opens anyway. The card process covers that case itself: it exits on the
// first byte it reads and re-sends that byte to the session (see the
// attach-banner subcommand), so at worst one key takes the long way round.
func (p *bannerPopup) dismiss(input []byte) {
	p.mu.Lock()
	if p.done {
		p.mu.Unlock()
		return
	}
	p.done = true
	client := p.client
	p.mu.Unlock()
	bannerLog.Debug("attach_banner_dismissed",
		slog.String("input", fmt.Sprintf("%q", input)),
		slog.Bool("was_open", client != ""),
		slog.Duration("after_attach", time.Since(p.started)))
	if client == "" {
		return // never opened; show() will see done and skip
	}
	// -C closes any popup on the client. It is a silent no-op once the card
	// has already timed out on its own.
	_ = exec.Command("tmux", "display-popup", "-C", "-c", client).Run()
}

// waitForClient polls list-clients for the client whose process is pid, which
// is the tmux attach we just spawned. Matching by pid rather than "most recent
// client" keeps a second terminal attached to the same session from getting
// our card.
func waitForClient(ctx context.Context, sessionName string, pid int) string {
	deadline := time.Now().Add(bannerClientWait)
	for {
		if c := clientByPID(sessionName, pid); c != "" {
			return c
		}
		if ctx.Err() != nil || time.Now().After(deadline) {
			return ""
		}
		time.Sleep(bannerClientPoll)
	}
}

func clientByPID(sessionName string, pid int) string {
	out, err := exec.Command("tmux", "list-clients", "-t", sessionName, "-F", "#{client_name} #{client_pid}").Output()
	if err != nil {
		return ""
	}
	return parseClientByPID(string(out), pid)
}

func parseClientByPID(listClients string, pid int) string {
	want := strconv.Itoa(pid)
	for _, line := range strings.Split(listClients, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == want {
			return fields[0]
		}
	}
	return ""
}
