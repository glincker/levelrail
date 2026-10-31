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

// Decide applies mode to a scan result with the default fail-open behavior.
func Decide(mode GateMode, scan *ScanSummary, overrideReason string) Decision {
	return DecideStrict(mode, scan, overrideReason, false)
}

// DecideStrict is Decide with an explicit failClosed choice. A nil scan
// (scanner unavailable or failed) is allowed unless failClosed is set and the
// mode blocks. Then it is denied, and unknown severities count as critical.
// overrideReason is a non-empty operator reason that lets a blocked release through.
func DecideStrict(mode GateMode, scan *ScanSummary, overrideReason string, failClosed bool) Decision {
	if mode == GateOff || mode == "" {
		return Decision{Action: ActionAllow}
	}
	strict := failClosed && mode.blocking()
	if scan == nil {
		if strict {
			return Decision{Action: ActionBlock, Reason: "scan unavailable"}
		}
		return Decision{Action: ActionAllow, Reason: "scan did not complete, release allowed"}
	}
	crit := scan.Counts.Critical
	if strict {
		crit += scan.Counts.Unknown
	}
	high := gateHighCount(mode, scan)
	if crit+high == 0 {
		return Decision{Action: ActionAllow}
	}
	reason := fmt.Sprintf("%d critical vulnerabilities (%d fixable of %d total)", crit, scan.Fixable, scan.Counts.Total())
	if mode == GateBlockOnHigh {
		reason = fmt.Sprintf("%d critical and %d high vulnerabilities (%d fixable of %d total)", crit, high, scan.Fixable, scan.Counts.Total())
	}
	switch mode {
	case GateBlockOnCritical, GateBlockOnHigh:
		if overrideReason != "" {
			return Decision{Action: ActionOverride, Reason: reason + "; overridden: " + overrideReason}
		}
		return Decision{Action: ActionBlock, Reason: reason}
	default:
		return Decision{Action: ActionWarn, Reason: reason}
	}
}

func (m GateMode) blocking() bool { return m == GateBlockOnCritical || m == GateBlockOnHigh }

// gateHighCount is the high findings that count toward mode: only block_on_high looks below critical.
func gateHighCount(mode GateMode, scan *ScanSummary) int {
	if mode == GateBlockOnHigh {
		return scan.Counts.High
	}
	return 0
}
