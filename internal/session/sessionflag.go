package session

// SessionFlag is a manual marker the user cycles with the `u` key. It is a
// quiet visual overlay on the status dot only — it never affects automatic
// status detection, and a flagged session does not count as needing attention.
//
// The values are persisted (in the tool_data blob), so their numeric order is
// a stable contract: append new states, don't reorder existing ones.
type SessionFlag int

const (
	FlagNone       SessionFlag = iota // no marker; dot shows live status
	FlagUnread                        // bright green: "come back to this"
	FlagParkedRed                     // dim red: shelved / skip visually
	FlagParkedBlue                    // dim blue: shelved, second bucket
)

// Next returns the next flag in the u-key cycle:
// none → unread → parked-red → parked-blue → none.
func (f SessionFlag) Next() SessionFlag {
	switch f {
	case FlagNone:
		return FlagUnread
	case FlagUnread:
		return FlagParkedRed
	case FlagParkedRed:
		return FlagParkedBlue
	default:
		return FlagNone
	}
}

// IsSet reports whether any marker is applied.
func (f SessionFlag) IsSet() bool { return f != FlagNone }

// IsParked reports whether the flag is one of the shelved/dimmed states.
func (f SessionFlag) IsParked() bool {
	return f == FlagParkedRed || f == FlagParkedBlue
}
