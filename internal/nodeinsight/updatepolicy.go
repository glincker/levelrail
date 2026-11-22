package nodeinsight

import (
	"github.com/GLINCKER/levelrail/internal/updatewindow"
	"strconv"
	"strings"
	"time"
)

// Agent update channels. stable follows the control plane; hold-minor only
// auto applies patch releases, so minor and major jumps wait for an operator.
const (
	ChannelStable    = "stable"
	ChannelHoldMinor = "hold-minor"
)

// Window is a daily local time range for automatic updates; see updatewindow.
type Window = updatewindow.Window

// ParseWindow reads "HH:MM-HH:MM"; empty means any time.
func ParseWindow(s string) (Window, error) { return updatewindow.Parse(s) }

// ChannelAllows reports whether the channel lets target apply automatically
// over current. Unparseable versions never block: the decision falls back to
// the stable behavior so nobody is stuck on a odd tag.
func ChannelAllows(channel, current, target string) bool {
	if channel != ChannelHoldMinor {
		return true
	}
	cm, ok1 := majorMinor(current)
	tm, ok2 := majorMinor(target)
	return !ok1 || !ok2 || cm == tm
}

func majorMinor(v string) ([2]int, bool) {
	parts := strings.SplitN(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".", 3)
	if len(parts) < 2 {
		return [2]int{}, false
	}
	a, err1 := strconv.Atoi(parts[0])
	b, err2 := strconv.Atoi(parts[1])
	return [2]int{a, b}, err1 == nil && err2 == nil
}

// UpdatePolicy is everything that gates an automatic apply for one node.
type UpdatePolicy struct {
	Auto       bool
	WindowText string
	Channel    string
	Held       bool
	// Override applies this one decision regardless of the gates. It never
	// changes the Auto setting and never skips signature checks or rollback.
	Override bool
}

// Evaluate says whether the agent may apply target now, or why it waits.
func (p UpdatePolicy) Evaluate(current, target string, now time.Time) (bool, string) {
	if p.Override {
		return true, ""
	}
	if !p.Auto {
		return false, "automatic updates are off (nodes.agent_auto_update)"
	}
	if p.Held {
		return false, "this node is on hold"
	}
	if !ChannelAllows(p.Channel, current, target) {
		return false, "channel " + ChannelHoldMinor + " only auto applies patch updates; " + target + " needs an explicit apply"
	}
	if w, err := ParseWindow(p.WindowText); err == nil && !w.Open(now) {
		return false, "outside the maintenance window " + strings.TrimSpace(p.WindowText)
	}
	return true, ""
}
