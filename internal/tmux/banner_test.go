//go:build !windows

package tmux

import (
	"strings"
	"testing"
)

func TestBannerWidth(t *testing.T) {
	tests := []struct {
		name string
		b    AttachBanner
		cols int
		want int
	}{
		{"short title gets the minimum", AttachBanner{Title: "x"}, 120, bannerMinWidth},
		{"long title sets the width", AttachBanner{Title: strings.Repeat("a", 40)}, 120, 46},
		{"subtitle wins when longer", AttachBanner{Title: "t", Subtitle: strings.Repeat("g", 30)}, 120, 36},
		{"clamped to the client", AttachBanner{Title: strings.Repeat("a", 200)}, 80, 76},
		{"unknown client size does not clamp", AttachBanner{Title: strings.Repeat("a", 60)}, 0, 66},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bannerWidth(tt.b, tt.cols); got != tt.want {
				t.Errorf("bannerWidth = %d, want %d", got, tt.want)
			}
		})
	}
}

// The popup must exec the binary directly with the text in the environment.
// A title with quotes or a `;` must never reach a shell.
func TestBannerPopupArgsNeverShellQuotes(t *testing.T) {
	b := AttachBanner{Title: `it's; "weird" $(x)`, Subtitle: "grp/sub"}
	args := bannerPopupArgs("/dev/ttys004", "agent-desk_abc", "/usr/local/bin/agent-desk", b, 100)

	joined := strings.Join(args, "\x00")
	for _, want := range []string{
		"-c\x00/dev/ttys004",
		"-t\x00agent-desk_abc",
		"-e\x00" + BannerEnvTitle + "=" + b.Title,
		"-e\x00" + BannerEnvSubtitle + "=grp/sub",
		"-e\x00" + BannerEnvTarget + "=agent-desk_abc",
		"/usr/local/bin/agent-desk\x00" + BannerSubcommand,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q:\n%q", want, args)
		}
	}
	// Last two args are exe + verb, which is what makes tmux exec instead of `sh -c`.
	if args[len(args)-1] != BannerSubcommand {
		t.Errorf("last arg = %q, want the subcommand", args[len(args)-1])
	}
}

func TestRenderBanner(t *testing.T) {
	out := RenderBanner("work", "grp", 20, 3)
	lines := strings.Split(out, "\r\n")
	if len(lines) != 3 || lines[0] != "" {
		t.Fatalf("want blank line then two lines, got %q", lines)
	}
	if !strings.Contains(lines[1], "\x1b[1m"+strings.Repeat(" ", 8)+"work") {
		t.Errorf("title not bold and centred: %q", lines[1])
	}
	if !strings.Contains(lines[2], "\x1b[2m") || !strings.Contains(lines[2], "grp") {
		t.Errorf("subtitle not dimmed: %q", lines[2])
	}

	// No subtitle: no dimmed line at all, not an empty one.
	if got := strings.Count(RenderBanner("work", "", 20, 3), "\r\n"); got != 1 {
		t.Errorf("without subtitle want 1 line break, got %d", got)
	}

	// Too long for the box: truncated with an ellipsis, never wrapped.
	long := RenderBanner(strings.Repeat("a", 50), "", 10, 3)
	for _, l := range strings.Split(long, "\r\n") {
		if strings.Count(l, "a") > 10 {
			t.Errorf("line not truncated to 10 cells: %q", l)
		}
	}
	if !strings.Contains(long, "…") {
		t.Errorf("truncation should show an ellipsis: %q", long)
	}

	// A one-row popup interior shows the title and nothing else.
	if got := RenderBanner("t", "s", 20, 1); strings.Contains(got, "\r\n") {
		t.Errorf("one row should be one line: %q", got)
	}
}

func TestParseClientByPID(t *testing.T) {
	out := "/dev/ttys001 4242\n/dev/ttys007 5150\n\n"
	if got := parseClientByPID(out, 5150); got != "/dev/ttys007" {
		t.Errorf("got %q", got)
	}
	if got := parseClientByPID(out, 1); got != "" {
		t.Errorf("unknown pid should be empty, got %q", got)
	}
}
