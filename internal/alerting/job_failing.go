package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/store"
)

// KindJobFailing sends one notice per failing job that opted into failure
// alerts, rate limited per rule and job. ResourceID must be service:<app>.
const KindJobFailing Kind = "job_failing"

// JobFailingSource claims jobs due a failure alert; *store.DB satisfies it.
type JobFailingSource interface {
	ClaimJobFailureAlerts(ctx context.Context, ruleID, service string, now, windowStart time.Time, limit int) ([]store.JobFailureAlert, error)
	ReleaseJobFailureAlert(ctx context.Context, ruleID, taskID string, claimedAt time.Time) error
}

// jobFailingResourcePrefix scopes a job_failing rule to one app.
const jobFailingResourcePrefix = "service:"

// SetJobFailing wires the job failure alert. window is read every tick so the
// live setting applies; limit bounds the jobs claimed per rule per tick.
func (e *Engine) SetJobFailing(src JobFailingSource, window func() time.Duration, limit func() int) {
	e.jobFailing, e.jobFailingWindow, e.jobFailingLimit = src, window, limit
}

// tickJobFailing sends through the rule's own channel via sendEvent, the same
// outbound path every other kind uses. The claim is the rate limit.
func (e *Engine) tickJobFailing(ctx context.Context, r Rule, now time.Time) error {
	if e.jobFailing == nil || e.jobFailingWindow == nil || e.jobFailingLimit == nil {
		return nil
	}
	window := e.jobFailingWindow()
	if window <= 0 {
		return nil
	}
	service, scoped := strings.CutPrefix(r.ResourceID, jobFailingResourcePrefix)
	if !scoped || service == "" {
		return nil
	}
	claimed, err := e.jobFailing.ClaimJobFailureAlerts(ctx, r.ID, service, now, now.Add(-window), e.jobFailingLimit())
	for _, a := range claimed {
		if serr := e.sendEvent(ctx, Event{Rule: r, TaskFailureNotice: JobFailureNotice(a)}); serr != nil {
			// Undo the claim so the next tick retries instead of muting the window.
			if rerr := e.jobFailing.ReleaseJobFailureAlert(ctx, r.ID, a.TaskID, now); rerr != nil {
				e.logger.Warn("alerting: release job failure alert failed", slog.String("rule_id", r.ID), slog.String("error", rerr.Error()))
			}
		}
	}
	if err != nil {
		return fmt.Errorf("rule %q: %w", r.ID, err)
	}
	return nil
}

// JobFailureNotice names the job and its exit code only; run output and env
// values never reach a notification.
func JobFailureNotice(a store.JobFailureAlert) string {
	name := cleanAlertText(a.Name)
	if name == "" {
		name = cleanAlertText(a.TaskID)
	}
	code := "none"
	if a.ExitCode != nil {
		code = strconv.Itoa(*a.ExitCode)
	}
	return fmt.Sprintf("job %q on app %q failed (status %s, exit code %s)", name, cleanAlertText(a.ServiceName), cleanAlertText(a.Status), code)
}

// cleanAlertText drops control characters so a name cannot forge extra lines
// in a chat or email notice, and bounds its length.
func cleanAlertText(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			return -1
		}
		return r
	}, s)
	const maxBytes = 128
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
