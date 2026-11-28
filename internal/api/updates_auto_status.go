package api

import (
	"context"
	"time"

	"github.com/glincker/levelrail/internal/selfupgrade"
	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/upgradesched"
)

// Automatic update states shown on the status line.
const (
	autoStateOff      = "off"
	autoStatePaused   = "paused"
	autoStateBlocked  = "blocked"
	autoStateDegraded = "degraded"
	autoStateReady    = "ready"
)

// upgradeAutoStatus answers "is it working, and if not, why" in one object for the dashboard and the CLI.
type upgradeAutoStatus struct {
	State            string `json:"state"`
	Reason           string `json:"reason,omitempty"`
	CanApply         bool   `json:"can_apply"`
	LastCheckAt      string `json:"last_check_at,omitempty"`
	LastCheckError   string `json:"last_check_error,omitempty"`
	CheckFailures    int    `json:"check_failures"`
	NextCheckAt      string `json:"next_check_at,omitempty"`
	LastResult       string `json:"last_result,omitempty"`
	LastResultReason string `json:"last_result_reason,omitempty"`
	LastResultAt     string `json:"last_result_at,omitempty"`
	// LastGoodListAt is when a fresh release list was last read; ListAgeSeconds is its age now.
	LastGoodListAt string `json:"last_good_list_at,omitempty"`
	ListAgeSeconds int64  `json:"list_age_seconds,omitempty"`
	// ListSource is "api" or "fallback" (public release pages, no breaking-change notes).
	ListSource string `json:"list_source,omitempty"`
	// SignaturePinned is false until a release signature was verified here, so the first update relies on the checksum.
	SignaturePinned *bool `json:"signature_pinned,omitempty"`
}

// upgradeAutoStatusFor derives the status from the policy in force, this host's ability to apply an
// upgrade, the scheduler's own check log and the newest schedule outcome.
func (rt *Router) upgradeAutoStatusFor(ctx context.Context, row store.UpgradePolicy) *upgradeAutoStatus {
	out := &upgradeAutoStatus{State: autoStateReady}
	out.CanApply, out.Reason = true, ""
	if rt.selfUpgrade == nil {
		out.CanApply, out.Reason = false, "self-upgrade is not configured on this control plane"
	} else {
		out.CanApply, out.Reason = rt.selfUpgrade.canLaunch()
	}
	if s := rt.UpgradeScheduler(); s != nil {
		cs := s.CheckStatus()
		out.CheckFailures, out.LastCheckError = cs.Failures, cs.LastError
		if !cs.LastCheckAt.IsZero() {
			out.LastCheckAt = cs.LastCheckAt.UTC().Format(time.RFC3339)
		}
		if !cs.LastGoodAt.IsZero() {
			out.LastGoodListAt = cs.LastGoodAt.UTC().Format(time.RFC3339)
			out.ListAgeSeconds = max(0, int64(time.Since(cs.LastGoodAt).Seconds()))
			out.ListSource = cs.Source
		}
		if !cs.NextCheckAt.IsZero() && cs.Failures > 0 {
			out.NextCheckAt = cs.NextCheckAt.UTC().Format(time.RFC3339)
		}
	}
	if rt.dataDir != "" {
		_, pinned, _ := selfupgrade.ReadPin(rt.dataDir)
		out.SignaturePinned = &pinned
	}
	if list, err := rt.upgradeSchedule.store.ListUpgradeSchedules(ctx, 1); err == nil && len(list) > 0 {
		last := list[0]
		out.LastResult, out.LastResultReason = last.State, last.LastReason
		out.LastResultAt = firstNonEmpty(last.FinishedAt, last.FiredAt, last.CreatedAt)
	}
	switch {
	case row.Mode == upgradesched.ModeOff:
		out.State, out.Reason = autoStateOff, ""
	case row.Paused || time.Now().Before(upgradesched.PausedUntil(row)):
		out.State, out.Reason = autoStatePaused, ""
	case !out.CanApply:
		out.State = autoStateBlocked
	case !rt.controlPlaneBackupConfigured():
		out.State, out.CanApply = autoStateBlocked, false
		out.Reason = "Automatic updates are held: no control plane backup is configured. Configure one (see docs/control-plane-backup.md)."
	case out.CheckFailures > 0:
		out.State, out.Reason = autoStateDegraded, out.LastCheckError
	default:
		out.Reason = ""
	}
	return out
}
