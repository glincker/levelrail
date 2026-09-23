// Package probe implements readiness and liveness checks the controller
// runs against a container: an HTTP(S) request to its published port, or
// a command exec'd inside it. See ADR 002 for why the controller probes
// rather than trusting Docker's own HEALTHCHECK state.
//
// WaitReady polls until a freshly started container is ready, gating a
// deploy's cutover. Check runs a single attempt, for the level-triggered
// liveness check, where the reconcile loop itself is the retry loop.
package probe

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Supported HTTP probe schemes.
const (
	SchemeHTTP  = "http"
	SchemeHTTPS = "https"
)

// Config is one probe's settings. A non-empty Exec makes it an exec probe;
// otherwise it is an HTTP probe against Path. Zero Interval or Timeout
// means "not specified" and falls back to the Prober's Limits.
type Config struct {
	Path          string
	Scheme        string
	Host          string
	TLSSkipVerify bool
	// FollowRedirects nil keeps the historical behavior (follow, bounded by
	// Limits.MaxRedirects); an explicit false judges the 3xx itself.
	FollowRedirects *bool
	ExpectedStatus  string
	Exec            []string
	Interval        time.Duration
	Timeout         time.Duration
}

// IsExec reports whether c is an exec probe.
func (c Config) IsExec() bool { return len(c.Exec) > 0 }

func (c Config) followRedirects() bool { return c.FollowRedirects == nil || *c.FollowRedirects }

// Validate checks c for contradictory or malformed settings.
func (c Config) Validate() error {
	if c.IsExec() {
		return c.validateExec()
	}
	if c.Path == "" {
		return errors.New("path (for an HTTP probe) or exec (for a command probe) is required")
	}
	if !strings.HasPrefix(c.Path, "/") {
		return fmt.Errorf("path %q must start with /", c.Path)
	}
	switch c.Scheme {
	case "", SchemeHTTP, SchemeHTTPS:
	default:
		return fmt.Errorf("scheme %q must be http or https", c.Scheme)
	}
	if c.TLSSkipVerify && c.Scheme != SchemeHTTPS {
		return errors.New("tls_skip_verify only applies to scheme: https")
	}
	if c.Host != "" && strings.ContainsAny(c.Host, "/ \t") {
		return fmt.Errorf("host %q must be a bare hostname or host:port, with no scheme or path", c.Host)
	}
	if _, err := ParseStatusSet(c.ExpectedStatus); err != nil {
		return err
	}
	if c.Interval < 0 || c.Timeout < 0 {
		return errors.New("interval and timeout must not be negative")
	}
	return nil
}

func (c Config) validateExec() error {
	if c.Path != "" {
		return errors.New("set either path (HTTP probe) or exec (command probe), not both")
	}
	if c.Scheme != "" || c.Host != "" || c.TLSSkipVerify || c.FollowRedirects != nil || c.ExpectedStatus != "" {
		return errors.New("scheme, host, tls_skip_verify, follow_redirects and expected_status apply to HTTP probes only, not exec")
	}
	if strings.TrimSpace(c.Exec[0]) == "" {
		return errors.New("exec: the command must not be empty")
	}
	if c.Interval < 0 || c.Timeout < 0 {
		return errors.New("interval and timeout must not be negative")
	}
	return nil
}

// Target identifies what a probe runs against: Addr (host:port) for an
// HTTP probe, ContainerID for an exec probe.
type Target struct {
	Addr        string
	ContainerID string
}

// Executor runs cmd inside a container and reports its exit code and
// combined output. err is for failures to run the command at all.
type Executor interface {
	ExecProbe(ctx context.Context, containerID string, cmd []string) (exitCode int, output string, err error)
}

// FailureKind classifies why a probe attempt failed.
type FailureKind string

// Failure kinds.
const (
	FailureStatus       FailureKind = "UnexpectedStatus"
	FailureRedirect     FailureKind = "UnexpectedRedirect"
	FailureTooManyHops  FailureKind = "TooManyRedirects"
	FailureConnect      FailureKind = "ConnectionFailed"
	FailureTLS          FailureKind = "TLSVerifyFailed"
	FailureTimeout      FailureKind = "Timeout"
	FailureExitCode     FailureKind = "ExecNonZeroExit"
	FailureExecError    FailureKind = "ExecFailed"
	FailureExecNotReady FailureKind = "ExecUnavailable"
)

// Failure is one failed probe attempt. Its Error text is written for an
// operator: it names the request or command and exactly what went wrong.
type Failure struct {
	Kind   FailureKind
	Reason string
	Err    error
}

func (f *Failure) Error() string { return f.Reason }

func (f *Failure) Unwrap() error { return f.Err }

// Prober runs probes with shared transport and limits.
type Prober struct {
	client *http.Client
	exec   Executor
	limits Limits
}

// New builds a Prober. A nil client means http.DefaultClient; a nil exec
// makes every exec probe fail with FailureExecNotReady.
func New(client *http.Client, exec Executor, limits Limits) *Prober {
	if client == nil {
		client = http.DefaultClient
	}
	return &Prober{client: client, exec: exec, limits: limits.withDefaults()}
}

// Check runs exactly one probe attempt, bounded by cfg.Timeout.
// cfg.Interval is ignored: a caller wanting repetition owns it.
func (p *Prober) Check(ctx context.Context, t Target, cfg Config) error {
	return p.attempt(ctx, t, cfg)
}

// WaitReady retries cfg on cfg.Interval until an attempt succeeds or ctx
// is done, then returns ctx's error wrapped with the last failure.
func (p *Prober) WaitReady(ctx context.Context, t Target, cfg Config) error {
	interval := cfg.Interval
	if interval <= 0 {
		interval = p.limits.DefaultInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastErr error
	for {
		err := p.attempt(ctx, t, cfg)
		if err == nil {
			return nil
		}
		// An attempt cut short by ctx itself says nothing about the app,
		// so the previous attempt's reason is the more useful one to report.
		if lastErr == nil || ctx.Err() == nil {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("never became ready (%w), last attempt: %w", ctx.Err(), lastErr)
		case <-ticker.C:
		}
	}
}

func (p *Prober) attempt(ctx context.Context, t Target, cfg Config) error {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = p.limits.DefaultTimeout
	}
	if cfg.IsExec() {
		return p.execAttempt(ctx, t.ContainerID, cfg.Exec, timeout)
	}
	return p.httpAttempt(ctx, t.Addr, cfg, timeout)
}
