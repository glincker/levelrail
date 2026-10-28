// Package cigate holds a deploy until the CI checks on a commit conclude.
package cigate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// Environment variables that tune the gate.
const (
	EnvTimeout      = "APP_CI_GATE_TIMEOUT"
	EnvPollInterval = "APP_CI_GATE_POLL_INTERVAL"
	EnvNoChecksWait = "APP_CI_GATE_NO_CHECKS_WAIT"
)

// Defaults used when the environment leaves a value unset or invalid.
const (
	DefaultTimeout      = 15 * time.Minute
	DefaultPollInterval = 15 * time.Second
	DefaultNoChecksWait = time.Minute
	maxBackoffFactor    = 4
)

// State is the conclusion of one check.
type State int

// Check states.
const (
	Pending State = iota
	Passed
	Failed
)

// Check is one check run or commit status as the provider reports it.
type Check struct {
	Name  string
	State State
}

// Provider reads the checks reported for a commit.
type Provider interface {
	Checks(ctx context.Context, sha string) ([]Check, error)
}

// RateLimitError tells the gate to wait before the next read.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("cigate: provider rate limit, retry after %s", e.RetryAfter)
}

// Policy picks which checks the gate waits for.
type Policy struct {
	// Required names the checks that must pass; empty means every check seen.
	Required []string
	// IgnorePrefixes drops checks the platform posts itself, which would deadlock the gate.
	IgnorePrefixes []string
}

// Outcome is the gate verdict.
type Outcome string

// Verdicts.
const (
	OutcomePass    Outcome = "pass"
	OutcomeFail    Outcome = "fail"
	OutcomeTimeout Outcome = "timeout"
)

// Result carries the verdict and the checks that decided it.
type Result struct {
	Outcome Outcome
	Failing []string
	Pending []string
}

// Reason is a one-line explanation for logs and delivery notes.
func (r Result) Reason() string {
	switch r.Outcome {
	case OutcomeFail:
		return "CI checks failed: " + strings.Join(r.Failing, ", ")
	case OutcomeTimeout:
		if len(r.Pending) == 0 {
			return "timed out waiting for CI checks"
		}
		return "timed out waiting for CI checks: " + strings.Join(r.Pending, ", ")
	}
	return "CI checks passed"
}

// Config tunes polling. Sleep is replaceable so tests need no real time.
type Config struct {
	Timeout      time.Duration
	PollInterval time.Duration
	NoChecksWait time.Duration
	Sleep        func(ctx context.Context, d time.Duration) error
	Now          func() time.Time
}

// ConfigFromEnv reads the gate settings, falling back to the defaults.
func ConfigFromEnv() Config {
	return Config{
		Timeout:      envDuration(EnvTimeout, DefaultTimeout),
		PollInterval: envDuration(EnvPollInterval, DefaultPollInterval),
		NoChecksWait: envDuration(EnvNoChecksWait, DefaultNoChecksWait),
	}
}

func envDuration(key string, def time.Duration) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(key)))
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func (c Config) withDefaults() Config {
	if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	if c.PollInterval <= 0 {
		c.PollInterval = DefaultPollInterval
	}
	if c.NoChecksWait <= 0 {
		c.NoChecksWait = DefaultNoChecksWait
	}
	if c.Sleep == nil {
		c.Sleep = sleepCtx
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return c
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Wait polls p until every gated check concludes or the timeout passes.
// A failed check ends the wait at once. Read errors are retried until the
// timeout, so a flaky provider never reads as a pass.
func Wait(ctx context.Context, p Provider, sha string, pol Policy, cfg Config) (Result, error) {
	cfg = cfg.withDefaults()
	start := cfg.Now()
	interval := cfg.PollInterval
	var last Result
	for {
		checks, err := p.Checks(ctx, sha)
		wait := interval
		if err == nil {
			res, done := evaluate(checks, pol, cfg.Now().Sub(start) >= cfg.NoChecksWait)
			last = res
			if done {
				return res, nil
			}
			if interval < cfg.PollInterval*maxBackoffFactor {
				interval += cfg.PollInterval / 2
			}
		} else {
			var rl *RateLimitError
			if errors.As(err, &rl) && rl.RetryAfter > wait {
				wait = rl.RetryAfter
			}
		}
		if cfg.Now().Add(wait).Sub(start) > cfg.Timeout {
			last.Outcome = OutcomeTimeout
			return last, nil
		}
		if err := cfg.Sleep(ctx, wait); err != nil {
			return Result{}, fmt.Errorf("cigate: wait for checks: %w", err)
		}
	}
}

// evaluate returns the verdict so far and whether it is final.
func evaluate(all []Check, pol Policy, noChecksGraceOver bool) (Result, bool) {
	seen := map[string]State{}
	for _, c := range all {
		if ignored(c.Name, pol.IgnorePrefixes) {
			continue
		}
		// A rerun reports the same name again; a failure must not be hidden by an earlier pass.
		if prev, ok := seen[c.Name]; ok && prev == Failed {
			continue
		}
		seen[c.Name] = c.State
	}
	gated := map[string]bool{}
	if len(pol.Required) == 0 {
		for n := range seen {
			gated[n] = true
		}
	}
	for _, n := range pol.Required {
		gated[n] = true
	}
	var res Result
	for n := range gated {
		st, ok := seen[n]
		switch {
		case ok && st == Failed:
			res.Failing = append(res.Failing, n)
		case !ok || st == Pending:
			res.Pending = append(res.Pending, n)
		}
	}
	sort.Strings(res.Failing)
	sort.Strings(res.Pending)
	switch {
	case len(res.Failing) > 0:
		res.Outcome = OutcomeFail
		return res, true
	case len(res.Pending) > 0:
		return res, false
	case len(gated) == 0 && !noChecksGraceOver:
		return res, false
	}
	res.Outcome = OutcomePass
	return res, true
}

func ignored(name string, prefixes []string) bool {
	for _, p := range prefixes {
		if p != "" && strings.HasPrefix(strings.ToLower(name), strings.ToLower(p)) {
			return true
		}
	}
	return false
}
