package supplychain

import "fmt"

// Gate actions stored on a record.
const (
	ActionAllow    = "allow"
	ActionWarn     = "warn"
	ActionBlock    = "block"
	ActionOverride = "override"
)

// Decision is what the gate concluded for one release.
type Decision struct {
	Action string
	Reason string
}

// Blocked reports whether the release must not go live.
func (d Decision) Blocked() bool { return d.Action == ActionBlock }

// Decide applies mode to a scan result. A nil scan (scanner unavailable or
// failed) never blocks: the gate fails open and says so. overrideReason is a
// non-empty operator reason that lets a blocked release through.
func Decide(mode GateMode, scan *ScanSummary, overrideReason string) Decision {
	if mode == GateOff || mode == "" {
		return Decision{Action: ActionAllow}
	}
	if scan == nil {
		return Decision{Action: ActionAllow, Reason: "scan did not complete, release allowed"}
	}
	crit := scan.Counts.Critical
	if crit == 0 {
		return Decision{Action: ActionAllow}
	}
	reason := fmt.Sprintf("%d critical vulnerabilities (%d fixable of %d total)", crit, scan.Fixable, scan.Counts.Total())
	switch mode {
	case GateBlockOnCritical:
		if overrideReason != "" {
			return Decision{Action: ActionOverride, Reason: reason + "; overridden: " + overrideReason}
		}
		return Decision{Action: ActionBlock, Reason: reason}
	default:
		return Decision{Action: ActionWarn, Reason: reason}
	}
}
