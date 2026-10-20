package profile

import (
	"fmt"
	"time"
)

// Guard codes returned by Check.
const (
	GuardNodesEnrolled    = "nodes_enrolled"
	GuardPluginsRunning   = "plugins_running"
	GuardRetentionShorter = "retention_shorter"
)

// Facts is the live state a move is checked against.
type Facts struct {
	RemoteNodes    int
	EnabledPlugins int
	// MeshStops is true when the move turns the effective mesh from on to off.
	MeshStops bool
	// Retention is the retention in force now, NewRetention the one the target profile would use.
	Retention    time.Duration
	NewRetention time.Duration
}

// Problem is one reason a move needs attention. Blocking problems refuse the
// move; the rest need an explicit confirm because the next sweep deletes rows.
type Problem struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Blocking bool   `json:"blocking"`
}

// Check lists what moving from one profile to another would cost. Moving up
// never has a problem. No move deletes rows itself: gated routes return 404
// and loops stop, but a shorter retention is swept at the next pass.
func Check(from, to Profile, f Facts) []Problem {
	if to.Rank() >= from.Rank() {
		return nil
	}
	var out []Problem
	if to == Minimal && f.RemoteNodes > 0 {
		out = append(out, Problem{GuardNodesEnrolled, fmt.Sprintf("%d remote node(s) are enrolled and minimal binds the agent listener to loopback. Remove them first, or choose standard.", f.RemoteNodes), true})
	} else if f.MeshStops && f.RemoteNodes > 0 {
		out = append(out, Problem{GuardNodesEnrolled, fmt.Sprintf("%d remote node(s) use the mesh, which this profile turns off. Remove them first, or pin the mesh on in Network settings.", f.RemoteNodes), true})
	}
	if f.EnabledPlugins > 0 {
		out = append(out, Problem{GuardPluginsRunning, fmt.Sprintf("%d plugin(s) are enabled and would stop being supervised. Disable them first.", f.EnabledPlugins), true})
	}
	if f.NewRetention > 0 && f.NewRetention < f.Retention {
		out = append(out, Problem{GuardRetentionShorter, "A shorter metrics and log retention deletes older samples at the next sweep. Confirm to continue.", false})
	}
	return out
}

// Blocked reports whether any problem refuses the move.
func Blocked(ps []Problem) bool {
	for _, p := range ps {
		if p.Blocking {
			return true
		}
	}
	return false
}

// NeedsConfirm reports whether the move is allowed only with confirm set.
func NeedsConfirm(ps []Problem) bool {
	for _, p := range ps {
		if !p.Blocking {
			return true
		}
	}
	return false
}
