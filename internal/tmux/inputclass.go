//go:build !windows
// +build !windows

package tmux

// InputClassifier tells typed keys apart from everything else a terminal
// puts on stdin: replies to queries (device attributes, colours, XTVERSION),
// focus events, mouse reports. It keeps state ACROSS reads, which is the
// whole point: a burst of replies to tmux's attach-time queries is longer
// than one read, so the second chunk starts in the middle of a sequence with
// an ordinary byte. Judging each chunk by its first byte took that for a
// keystroke and dismissed the landing card almost as soon as it was drawn.
//
// Zero value is ready to use.
type InputClassifier struct {
	state  classState
	params []byte // CSI parameter/intermediate bytes seen so far
}

type classState int

const (
	stNormal    classState = iota
	stEsc                  // saw ESC
	stCSI                  // inside ESC [
	stSS3                  // inside ESC O (F1–F4, application-mode arrows)
	stString               // inside OSC / DCS / APC / PM / SOS
	stStringEsc            // inside a string and saw ESC (ST is ESC \)
)

// Feed consumes one chunk and reports whether a person typed anything in it.
func (c *InputClassifier) Feed(b []byte) (typed bool) {
	for _, by := range b {
		if c.feedByte(by) {
			typed = true
		}
	}
	return typed
}

func (c *InputClassifier) feedByte(b byte) bool {
	switch c.state {
	case stNormal:
		if b == 0x1b {
			c.state = stEsc
			return false
		}
		return true // printable or control byte: a key
	case stEsc:
		switch b {
		case '[':
			c.state, c.params = stCSI, c.params[:0]
			return false
		case 'O':
			c.state = stSS3
			return false
		case ']', 'P', '_', '^', 'X':
			c.state = stString
			return false
		case 0x1b:
			return true // ESC ESC: someone is mashing Escape
		default:
			c.state = stNormal
			return true // Alt+key
		}
	case stSS3:
		c.state = stNormal
		return true
	case stCSI:
		if b >= 0x40 && b <= 0x7e {
			c.state = stNormal
			return csiIsKey(c.params, b)
		}
		if len(c.params) < 64 {
			c.params = append(c.params, b)
		}
		return false
	case stString:
		switch b {
		case 0x07: // BEL terminates OSC
			c.state = stNormal
		case 0x1b:
			c.state = stStringEsc
		}
		return false
	case stStringEsc:
		if b == '\\' {
			c.state = stNormal // ST
		} else {
			c.state = stString
		}
		return false
	}
	return false
}

// csiIsKey decides a complete CSI sequence. Keys: arrows (A–D), Home/End
// (H/F), keypad (~ with a number), and modifier forms of those. Not keys:
// private-parameter reports (? > = for DA2/DA3/mode reports, < for SGR
// mouse), DA1 (c), cursor position (R), device status (n), focus (I/O),
// and the kitty keyboard flags report (u with ?).
func csiIsKey(params []byte, final byte) bool {
	if len(params) > 0 {
		switch params[0] {
		case '?', '>', '=', '<':
			return false
		}
	}
	switch final {
	case 'c', 'R', 'n':
		return false
	case 'I', 'O':
		return false // focus in / focus out
	case 'M', 'm':
		// X10 mouse (CSI M + 3 bytes) is not sent by tmux's mouse modes;
		// with params it is an SGR report already excluded above, or a
		// modifier form of nothing we know. Treat as not typed.
		return false
	}
	return true
}
