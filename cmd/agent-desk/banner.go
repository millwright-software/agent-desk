package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/millwright-software/agent-desk/internal/tmux"
	"golang.org/x/term"
)

// attachBannerSubcommand is the verb tmux.bannerPopupArgs invokes; keep the
// two in sync (a test in internal/tmux pins the string).
const attachBannerSubcommand = tmux.BannerSubcommand

// handleAttachBanner draws the landing card inside a tmux popup and holds it
// for the requested time, or until a key arrives. It is only ever run by
// agent-desk itself (see tmux.AttachBanner); the text arrives in the
// environment so no shell is involved.
//
// While a tmux popup is open, keys go to the popup, not the pane. The attach
// loop normally closes the card before forwarding the first key, but if one
// slips through to us we exit at once (tmux then closes the popup) and re-send
// the bytes to the session so nothing typed is lost.
func handleAttachBanner() {
	title := os.Getenv(tmux.BannerEnvTitle)
	subtitle := os.Getenv(tmux.BannerEnvSubtitle)
	target := os.Getenv(tmux.BannerEnvTarget)
	ms, _ := strconv.Atoi(os.Getenv(tmux.BannerEnvMillis))
	if ms <= 0 {
		ms = 1000
	}

	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || cols <= 0 {
		cols, rows = 40, 3
	}

	fmt.Print("\x1b[?25l") // no cursor blinking in the card
	fmt.Print(tmux.RenderBanner(title, subtitle, cols, rows))

	key := make(chan []byte, 1)
	if oldState, err := term.MakeRaw(int(os.Stdin.Fd())); err == nil {
		defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()
		go func() {
			buf := make([]byte, 32)
			if n, err := os.Stdin.Read(buf); err == nil && n > 0 {
				key <- buf[:n]
			}
		}()
	}

	select {
	case <-time.After(time.Duration(ms) * time.Millisecond):
	case b := <-key:
		if target != "" {
			_ = exec.Command("tmux", "send-keys", "-t", target, "-l", "--", string(b)).Run()
		}
	}
}
