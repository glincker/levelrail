// Package upgradesched runs scheduled and automatic control plane upgrades.
// It decides when to start an upgrade and whether it is safe to; the upgrade
// itself is the existing host-run self-upgrade engine (internal/selfupgrade),
// started through Host. Every decision is level-triggered from persisted
// state, so a restart at any point resumes instead of repeating.
package upgradesched

import (
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/semver"
)

// Policy modes.
const (
	ModeOff   = "off"
	ModePatch = "patch"
	ModeAny   = "any"
)

// Window bounds, so a typo cannot make a window that never closes or never fits an upgrade.
const (
	MinWindowDuration = 10 * time.Minute
	MaxWindowDuration = 24 * time.Hour
	MaxMinReleaseAge  = 90
)

// Window is a recurring maintenance window: it opens at each cron match in
// Timezone and stays open for Duration. It reuses the alerting window model
// so cron and daylight saving semantics are the ones operators already know.
type Window struct {
	Cron     string
	Duration time.Duration
	Timezone string
}

func (w Window) model() alerting.MaintenanceWindow {
	return alerting.MaintenanceWindow{Name: "upgrade window", Cron: w.Cron, Duration: w.Duration, Timezone: w.Timezone, Scope: alerting.ScopeAll, Enabled: true}
}

// Validate checks the cron expression, duration and timezone.
func (w Window) Validate() error {
	if w.Duration < MinWindowDuration || w.Duration > MaxWindowDuration {
		return fmt.Errorf("window duration must be between %s and %s", MinWindowDuration, MaxWindowDuration)
	}
	return w.model().Validate()
}

// Span is one concrete opening of a window.
type Span struct {
	Start time.Time
	End   time.Time
}

// Next returns the window opening to use at now: the current one when the
// window is open, otherwise the next one.
func (w Window) Next(now time.Time) (Span, bool, error) {
	st, err := w.model().StateAt(now)
	if err != nil {
		return Span{}, false, err
	}
	if st.Active {
		return Span{Start: now, End: st.End}, true, nil
	}
	if st.NextStart.IsZero() {
		return Span{}, false, nil
	}
	return Span{Start: st.NextStart, End: st.NextStart.Add(w.Duration)}, true, nil
}

// Policy is the automatic upgrade policy.
type Policy struct {
	Mode              string
	Window            Window
	MinReleaseAgeDays int
	Paused            bool
	NotifyChannelIDs  []string
}

// Validate checks a policy before it is saved. A window is required once
// automatic upgrades are on, since "any time" is exactly what it must avoid.
func (p Policy) Validate() error {
	switch p.Mode {
	case ModeOff, ModePatch, ModeAny:
	default:
		return fmt.Errorf("mode must be %q, %q or %q", ModeOff, ModePatch, ModeAny)
	}
	if p.MinReleaseAgeDays < 0 || p.MinReleaseAgeDays > MaxMinReleaseAge {
		return fmt.Errorf("minimum release age must be between 0 and %d days", MaxMinReleaseAge)
	}
	if p.Mode != ModeOff || p.Window.Cron != "" {
		if p.Window.Cron == "" {
			return errors.New("a maintenance window is required for automatic upgrades")
		}
		if err := p.Window.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Active reports whether the policy may start upgrades on its own.
func (p Policy) Active() bool { return p.Mode != ModeOff && !p.Paused }

// Candidate is a release being considered.
type Candidate struct {
	Tag         string
	PublishedAt time.Time
}

// Verdict is the result of weighing a candidate against a policy.
type Verdict struct {
	OK     bool
	Code   string
	Reason string
	// WaitUntil is set when the only thing missing is time.
	WaitUntil time.Time
}

// Verdict codes.
const (
	VerdictEligible    = "eligible"
	VerdictNotNewer    = "not_newer"
	VerdictOff         = "off"
	VerdictPaused      = "paused"
	VerdictNotPatch    = "not_patch"
	VerdictMajor       = "major_jump"
	VerdictBreaking    = "breaking_changes"
	VerdictNoNotes     = "notes_unavailable"
	VerdictTooYoung    = "too_young"
	VerdictNoWindow    = "no_window"
	VerdictPriorFailed = "prior_failed"
)

// Eligible decides whether the policy may apply cand on its own. Anything
// that needs a human (breaking changes, a major jump, unreadable notes) is
// never eligible: automatic upgrades only take releases nobody needs to read.
func Eligible(p Policy, current string, cand Candidate, breakingNeedAck int, notesAvailable bool, now time.Time) Verdict {
	if cmp, ok := semver.Compare(cand.Tag, current); !ok || cmp <= 0 {
		return Verdict{Code: VerdictNotNewer, Reason: "no newer release"}
	}
	if p.Mode == ModeOff {
		return Verdict{Code: VerdictOff, Reason: "automatic upgrades are off"}
	}
	if p.Paused {
		return Verdict{Code: VerdictPaused, Reason: "automatic upgrades are paused"}
	}
	if p.Window.Cron == "" {
		return Verdict{Code: VerdictNoWindow, Reason: "no maintenance window is configured"}
	}
	if CrossesMajor(current, cand.Tag) {
		return Verdict{Code: VerdictMajor, Reason: fmt.Sprintf("%s is a new major version and needs a manual upgrade", cand.Tag)}
	}
	if p.Mode == ModePatch && !SamePatchLine(current, cand.Tag) {
		return Verdict{Code: VerdictNotPatch, Reason: fmt.Sprintf("%s is not a patch release of the running minor version", cand.Tag)}
	}
	if !notesAvailable {
		return Verdict{Code: VerdictNoNotes, Reason: "release notes could not be fetched, so breaking changes cannot be ruled out"}
	}
	if breakingNeedAck > 0 {
		return Verdict{Code: VerdictBreaking, Reason: fmt.Sprintf("%s has %d breaking change(s) that need your acknowledgement", cand.Tag, breakingNeedAck)}
	}
	if p.MinReleaseAgeDays > 0 && !cand.PublishedAt.IsZero() {
		ready := cand.PublishedAt.Add(time.Duration(p.MinReleaseAgeDays) * 24 * time.Hour)
		if now.Before(ready) {
			return Verdict{Code: VerdictTooYoung, Reason: fmt.Sprintf("%s is waiting out the %d day minimum release age", cand.Tag, p.MinReleaseAgeDays), WaitUntil: ready}
		}
	}
	return Verdict{OK: true, Code: VerdictEligible, Reason: fmt.Sprintf("%s is eligible", cand.Tag)}
}

// CrossesMajor reports whether target has a different major version than current.
func CrossesMajor(current, target string) bool {
	c, ok1 := majorMinor(current)
	t, ok2 := majorMinor(target)
	return ok1 && ok2 && c[0] != t[0]
}

// SamePatchLine reports whether target differs from current only in the
// patch number.
func SamePatchLine(current, target string) bool {
	c, ok1 := majorMinor(current)
	t, ok2 := majorMinor(target)
	return ok1 && ok2 && c == t
}

func majorMinor(v string) ([2]int, bool) {
	var major, minor int
	s := v
	if len(s) > 0 && s[0] == 'v' {
		s = s[1:]
	}
	if n, err := fmt.Sscanf(s, "%d.%d", &major, &minor); err != nil || n != 2 {
		return [2]int{}, false
	}
	return [2]int{major, minor}, true
}

// PolicyFromStore converts the persisted row.
func PolicyFromStore(row store.UpgradePolicy, channelIDs []string) Policy {
	return Policy{
		Mode: row.Mode, MinReleaseAgeDays: row.MinReleaseAgeDays, Paused: row.Paused, NotifyChannelIDs: channelIDs,
		Window: Window{Cron: row.WindowCron, Duration: time.Duration(row.WindowDurationSeconds) * time.Second, Timezone: row.WindowTimezone},
	}
}
