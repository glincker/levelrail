package takeover

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Boot actions.
const (
	BootNone     = "none"
	BootApply    = "apply"
	BootRollback = "rollback"
)

// BootDecision is what the process does with a stored run before it binds.
type BootDecision struct {
	Action string
	Reason string
	Run    Run
}

// DecideBoot maps a stored run to a boot action. A run only gets one boot
// with the new addresses before verification ends it: a second start while it
// is still unverified means the first one failed, so the old addresses return.
func DecideBoot(run Run) BootDecision {
	d := BootDecision{Action: BootNone, Run: run}
	switch run.State {
	case StatePreparing:
		d.Action, d.Reason = BootRollback, "the control plane stopped in the middle of the takeover"
	case StateApplying:
		if run.Boots == 0 {
			d.Action = BootApply
			return d
		}
		d.Action, d.Reason = BootRollback, "the control plane did not come up on the new ports, so the takeover was rolled back"
	case StateVerifying:
		d.Action, d.Reason = BootRollback, "the control plane restarted before verification finished"
	case StateDone:
		d.Action = BootApply
	case StateRollingBack:
		d.Action = BootNone
	}
	return d
}

// PrepareBoot loads the latest unfinished or finished-active run, records the
// decision (a boot counted, or the rollback intent) and returns it. A nil
// store or no run is BootNone.
func PrepareBoot(ctx context.Context, st Store, now time.Time) (BootDecision, error) {
	if st == nil {
		return BootDecision{Action: BootNone}, nil
	}
	runs, err := st.ListByState(ctx, StatePreparing, StateApplying, StateVerifying, StateDone)
	if err != nil {
		return BootDecision{Action: BootNone}, fmt.Errorf("takeover: load runs: %w", err)
	}
	if len(runs) == 0 {
		return BootDecision{Action: BootNone}, nil
	}
	d := DecideBoot(runs[len(runs)-1])
	switch d.Action {
	case BootApply:
		if d.Run.State == StateApplying {
			d.Run.Boots++
			d.Run.UpdatedAt = now
			if err := st.Save(ctx, d.Run); err != nil {
				return BootDecision{Action: BootNone}, fmt.Errorf("takeover: count boot: %w", err)
			}
		}
	case BootRollback:
		d.Run.State, d.Run.Error, d.Run.UpdatedAt = StateRollingBack, d.Reason, now
		d.Run.step(StepRestore, "Roll back", StepRunning, d.Reason, now)
		if err := st.Save(ctx, d.Run); err != nil {
			return BootDecision{Action: BootNone}, fmt.Errorf("takeover: record rollback: %w", err)
		}
	}
	return d, nil
}

var (
	hostCall   = regexp.MustCompile(`Host(?:SNI)?\(([^)]*)\)`)
	quotedHost = regexp.MustCompile("[`\"']([^`\"']+)[`\"']")
)

// TraefikHosts returns the hostnames a container's Traefik router labels
// claim, sorted and without duplicates. Regex and wildcard rules are skipped.
func TraefikHosts(labels map[string]string) []string {
	seen := map[string]bool{}
	for k, v := range labels {
		if !strings.HasPrefix(k, "traefik.http.routers.") || !strings.HasSuffix(k, ".rule") {
			continue
		}
		for _, call := range hostCall.FindAllStringSubmatch(v, -1) {
			for _, q := range quotedHost.FindAllStringSubmatch(call[1], -1) {
				h := strings.ToLower(strings.TrimSpace(q[1]))
				if h != "" && !strings.ContainsAny(h, "*{}") {
					seen[h] = true
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for h := range seen {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}
