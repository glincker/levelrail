package alerting

import (
	"context"
	"sync"
	"time"
)

// logRuleRuntime is the in-memory state log rules need between ticks: the
// evaluation cadence, the cooldown clock and the samples to attach when the
// rule's firing transition dispatches.
type logRuleRuntime struct {
	policy   LogRulePolicy
	throttle *domainHealthThrottle

	mu        sync.Mutex
	notice    map[string]string
	samples   map[string][]string
	lastFired map[string]time.Time
}

func newLogRuleRuntime() logRuleRuntime {
	return logRuleRuntime{
		policy:    LogRulePolicyFromEnv(),
		throttle:  newDomainHealthThrottle(),
		notice:    make(map[string]string),
		samples:   make(map[string][]string),
		lastFired: make(map[string]time.Time),
	}
}

func (l *logRuleRuntime) store(id, notice string, samples []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.notice[id], l.samples[id] = notice, samples
}

func (l *logRuleRuntime) take(id string) (string, []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.notice[id], l.samples[id]
}

// inCooldown records a new firing at now and reports whether the previous one
// was recent enough to suppress it.
func (l *logRuleRuntime) inCooldown(id string, cooldown time.Duration, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if last, ok := l.lastFired[id]; ok && cooldown > 0 && now.Sub(last) < cooldown {
		return true
	}
	l.lastFired[id] = now
	return false
}

// evaluateLogRule runs one log rule if its cadence allows. skip is true when
// the rule was throttled and nothing should be persisted this tick.
func (e *Engine) evaluateLogRule(ctx context.Context, r Rule, now time.Time) (next Rule, skip bool, err error) {
	if !e.logRule.throttle.ready(r.ID, e.logRule.policy.EvalEvery, now) {
		return r, true, nil
	}
	next, res, err := EvaluateLogRule(ctx, e.logs, r, e.logRule.policy, now)
	if err != nil {
		return r, false, err
	}
	if next.Firing && !r.Firing && r.LogRule != nil {
		cooldown := time.Duration(r.LogRule.CooldownSeconds) * time.Second
		if e.logRule.inCooldown(r.ID, cooldown, now) {
			next.Firing, next.FiringSince, next.PendingSince = false, nil, nil
		}
	}
	e.logRule.store(r.ID, res.Notice, res.Samples)
	return next, false, nil
}
