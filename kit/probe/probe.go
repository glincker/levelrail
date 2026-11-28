// Package probe runs readiness and liveness checks against a container: an
// HTTP(S) request to its address, or a command executed inside it. WaitReady
// polls until the target is ready, and Check runs a single attempt.
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
	// TCP makes it a connect probe against the target address, no HTTP.
	TCP      bool
	Interval time.Duration
	Timeout  time.Duration
	// MaxRedirects is the resolved redirect bound a Prober hands to a
	// RemoteChecker; 0 keeps the checker's default. Local attempts ignore it.
	MaxRedirects int
}

// IsExec reports whether c is an exec probe.
func (c Config) IsExec() bool { return len(c.Exec) > 0 }

func (c Config) followRedirects() bool { return c.FollowRedirects == nil || *c.FollowRedirects }

// Validate checks c for contradictory or malformed settings.
func (c Config) Validate() error {
	if c.TCP {
		if c.IsExec() || c.Path != "" {
			return errors.New("a tcp probe cannot also set path or exec")
		}
		return nil
	}
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
	// Remote, when set, runs a TCP or HTTP attempt on the host that owns Addr
	// instead of dialing it from this process.
	Remote RemoteChecker
	// FallbackAddr, when set on an exec probe target, is dialed over TCP if
	// the command cannot run at all (shell-less image).
	FallbackAddr string
}

// RemoteChecker runs one probe attempt on another host and returns the same
// *Failure an in-process attempt would, or nil on success.
type RemoteChecker interface {
	CheckRemote(ctx context.Context, addr string, cfg Config) error
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
	// FailureUnsupported marks a probe that cannot run in this image with no
	// port to fall back to; it is reported, never counted as a failure.
	FailureUnsupported FailureKind = "ProbeUnsupported"
)

// Failure is one failed probe attempt. Its Error text is written for an
// operator: it names the request or command and exactly what went wrong.
type Failure struct {
	Kind   FailureKind
	Reason string
	Err    error
}

// Error returns the human-readable failure reason.
func (f *Failure) Error() string { return f.Reason }

// Unwrap returns the underlying error, if any.
func (f *Failure) Unwrap() error { return f.Err }

// Prober runs probes with shared transport and limits.
type Prober struct {
	client    *http.Client
	exec      Executor
	limits    Limits
	onAttempt func(Attempt)
	// loopbackOnly refuses any connection or redirect to a non-loopback address.
	loopbackOnly bool
}

// ErrRemoteUnsupported is wrapped by a RemoteChecker that cannot run the
// attempt, so the Prober falls back to dialing the address itself.
var ErrRemoteUnsupported = errors.New("probe: remote checker unavailable")

// Option configures an optional Prober behavior.
type Option func(*Prober)

// Attempt is one individual probe attempt's outcome, reported via
// WithOnAttempt strictly as a side channel: it is built and delivered
// after attempt()'s own pass/fail decision is already made, and nothing
// it does can feed back into WaitReady's retry loop or Check's return
// value. Target is the same descriptive string a Failure's own Reason
// would use (e.g. "GET http://addr/healthz", or an exec command), so a
// caller rendering this needs no separate formatting.
type Attempt struct {
	Time       time.Time
	Target     string
	Latency    time.Duration
	Success    bool
	StatusCode int // HTTP status; 0 for an exec probe or a response-less HTTP failure
	ExitCode   int // exec exit code; 0 for an HTTP probe
	Error      string
}

// WithLoopbackOnly makes the Prober refuse any connection, redirect or name
// resolution that does not end at a loopback IP address.
func WithLoopbackOnly() Option {
	return func(p *Prober) { p.loopbackOnly = true }
}

// WithOnAttempt reports every individual probe attempt to fn, once per
// call to attempt() (so once per WaitReady retry, once per Check), as a
// side effect after that attempt's outcome is already decided. fn runs
// synchronously on the calling goroutine and must not block; its
// behavior can never change what WaitReady/Check return. Exists so an
// operator-facing recorder can see per-attempt detail (status code,
// latency) that a reconcile condition's own single Reason/Message
// summary never captures.
func WithOnAttempt(fn func(Attempt)) Option {
	return func(p *Prober) { p.onAttempt = fn }
}

// New builds a Prober. A nil client means http.DefaultClient; a nil exec
// makes every exec probe fail with FailureExecNotReady.
func New(client *http.Client, exec Executor, limits Limits, opts ...Option) *Prober {
	if client == nil {
		client = http.DefaultClient //nolint:forbidigo // probes target the local container, so the SSRF guard would block them
	}
	p := &Prober{client: client, exec: exec, limits: limits.withDefaults()}
	for _, opt := range opts {
		opt(p)
	}
	if p.loopbackOnly {
		p.client = loopbackClient()
	}
	return p
}

// report delivers one Attempt to p.onAttempt, if configured. Centralized
// here so httpAttempt/execAttempt each need only a single call site.
func (p *Prober) report(start time.Time, target string, statusCode, exitCode int, err error) {
	if p.onAttempt == nil {
		return
	}
	a := Attempt{
		Time:       start,
		Target:     target,
		Latency:    time.Since(start),
		Success:    err == nil,
		StatusCode: statusCode,
		ExitCode:   exitCode,
	}
	if err != nil {
		a.Error = err.Error()
	}
	p.onAttempt(a)
}

// Check runs exactly one probe attempt, bounded by cfg.Timeout.
// cfg.Interval is ignored: a caller wanting repetition owns it.
func (p *Prober) Check(ctx context.Context, t Target, cfg Config) error {
	return p.attempt(ctx, t, cfg)
}

// WaitReady retries cfg until an attempt succeeds or ctx is done, then
// returns ctx's error wrapped with the last failure. The first retries come
// quickly (limits.FirstInterval, doubling) and settle at cfg.Interval, so an
// app that is ready a moment after start is not held back by a slow cadence.
func (p *Prober) WaitReady(ctx context.Context, t Target, cfg Config) error {
	interval := cfg.Interval
	if interval <= 0 {
		interval = p.limits.DefaultInterval
	}
	wait := p.limits.FirstInterval
	if wait <= 0 {
		wait = DefaultFirstInterval
	}
	if wait > interval {
		wait = interval
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()

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
		case <-timer.C:
		}
		wait = min(wait*2, interval)
		timer.Reset(wait)
	}
}

func (p *Prober) attempt(ctx context.Context, t Target, cfg Config) error {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = p.limits.DefaultTimeout
	}
	if t.Remote != nil && !cfg.IsExec() {
		remoteCfg := cfg
		remoteCfg.Timeout, remoteCfg.MaxRedirects = timeout, p.limits.MaxRedirects
		start := time.Now()
		err := t.Remote.CheckRemote(ctx, t.Addr, remoteCfg)
		if !errors.Is(err, ErrRemoteUnsupported) {
			p.report(start, t.Addr, 0, 0, err)
			return err
		}
	}
	if cfg.TCP {
		return p.tcpAttempt(ctx, t.Addr, timeout)
	}
	if cfg.IsExec() {
		err := p.execAttempt(ctx, t.ContainerID, cfg.Exec, timeout)
		if err != nil && execCannotRun(err) {
			if t.FallbackAddr != "" {
				return p.tcpAttempt(ctx, t.FallbackAddr, timeout)
			}
			// A vanished container is a real failure, not an unsupported probe.
			if !strings.Contains(err.Error(), "no such container") {
				return p.unsupportedExec(t.ContainerID, cfg.Exec, err)
			}
		}
		return err
	}
	return p.httpAttempt(ctx, t.Addr, cfg, timeout)
}
