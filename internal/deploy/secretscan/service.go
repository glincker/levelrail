package secretscan

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Store persists policy, allow-list entries and run records; *SQLStore satisfies it.
type Store interface {
	GetPolicy(ctx context.Context, app string) (Policy, error)
	SetPolicy(ctx context.Context, app string, p Policy) error
	ListAllow(ctx context.Context, app string) ([]AllowEntry, error)
	AddAllow(ctx context.Context, e AllowEntry) error
	RemoveAllow(ctx context.Context, app, fingerprint string) (bool, error)
	SaveRun(ctx context.Context, r Run) error
	GetRun(ctx context.Context, attemptID string) (Run, error)
	DeleteApp(ctx context.Context, app string) error
}

// Input is what one deploy hands the scanner. Root is the build context
// (empty for image deploys) and Env only the plain, non-secret values.
type Input struct {
	App       string
	AttemptID string
	Root      string
	Env       map[string]string
}

// BlockedError is returned to the deploy pipeline when the policy blocks a release.
type BlockedError struct{ Reason string }

func (e *BlockedError) Error() string { return "secret scan blocked this release: " + e.Reason }

// Service scans deploy inputs and applies each app's policy.
type Service struct {
	store Store
	lim   Limits
	log   *slog.Logger
	now   func() time.Time
	scan  func(ctx context.Context, root string, lim Limits) (Result, error)
}

// Option customizes a Service.
type Option func(*Service)

// WithClock overrides the clock, for tests.
func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

// WithDirScanner swaps the directory scanner, for tests.
func WithDirScanner(f func(ctx context.Context, root string, lim Limits) (Result, error)) Option {
	return func(s *Service) { s.scan = f }
}

// New builds a Service.
func New(st Store, lim Limits, log *slog.Logger, opts ...Option) *Service {
	if log == nil {
		log = slog.Default()
	}
	s := &Service{store: st, lim: lim, log: log, now: time.Now, scan: ScanDir}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Limits returns the effective caps.
func (s *Service) Limits() Limits { return s.lim }

// BeforeDeploy scans in and applies the app's policy. Only a block returns
// an error. It runs on the deploy job goroutine, bounded by the scan timeout.
func (s *Service) BeforeDeploy(ctx context.Context, in Input) error {
	policy, err := s.store.GetPolicy(ctx, in.App)
	if err != nil {
		s.log.Warn("secretscan: load policy failed, using warn", slog.String("app", in.App), slog.String("error", err.Error()))
		policy = PolicyWarn
	}
	if policy == PolicyOff {
		return nil
	}
	run := s.evaluate(ctx, policy, in)
	if in.AttemptID != "" {
		if err := s.store.SaveRun(ctx, run); err != nil {
			s.log.Warn("secretscan: save run failed", slog.String("app", in.App), slog.String("attempt_id", in.AttemptID), slog.String("error", err.Error()))
		}
	}
	if run.Action == ActionBlock {
		return &BlockedError{Reason: run.Reason}
	}
	return nil
}

func (s *Service) evaluate(ctx context.Context, policy Policy, in Input) Run {
	run := Run{AttemptID: in.AttemptID, App: in.App, Policy: policy, Status: StatusComplete, Findings: []Finding{}, ScannedAt: s.now().UTC()}
	scanCtx, cancel := context.WithTimeout(ctx, s.lim.Timeout)
	defer cancel()

	envRes := ScanEnv(in.Env, s.lim)
	res := envRes
	var scanErr error
	if in.Root != "" {
		dirRes, err := s.scan(scanCtx, in.Root, s.lim)
		scanErr = err
		res.Findings = append(res.Findings, dirRes.Findings...)
		res.FilesScanned, res.FilesSkipped = dirRes.FilesScanned, dirRes.FilesSkipped
		res.Truncated = res.Truncated || dirRes.Truncated
	}
	allow, err := s.store.ListAllow(ctx, in.App)
	if err != nil && scanErr == nil {
		scanErr = fmt.Errorf("load allow-list: %w", err)
	}
	active := applyAllow(res.Findings, allow)
	run.Findings, run.FilesScanned, run.Truncated = res.Findings, res.FilesScanned, res.Truncated
	if run.Findings == nil {
		run.Findings = []Finding{}
	}
	run.Action, run.Reason, run.Status = decide(policy, active, scanErr, res.Truncated)
	if active > 0 && scanErr == nil {
		run.Reason = Describe(run.Reason, run.Findings)
	}
	if scanErr != nil {
		run.Error = scanErr.Error()
	}
	return run
}

// applyAllow marks allow-listed findings and returns the ones still active.
func applyAllow(f []Finding, allow []AllowEntry) int {
	set := make(map[string]bool, len(allow))
	for _, a := range allow {
		set[a.Fingerprint] = true
	}
	active := 0
	for i := range f {
		if set[f[i].Fingerprint] {
			f[i].Allowed = true
			continue
		}
		active++
	}
	return active
}

// decide turns a scan outcome into an action. A scan error never passes a
// block policy silently: it fails closed there and warns under warn.
func decide(policy Policy, active int, scanErr error, truncated bool) (action, reason, status string) {
	switch {
	case scanErr != nil:
		reason = "secret scan did not complete: " + scanErr.Error()
		status = StatusError
		if policy == PolicyBlock {
			return ActionBlock, reason, status
		}
		return ActionWarn, reason, status
	case active > 0:
		reason = fmt.Sprintf("%d suspected secret(s) found", active)
		if policy == PolicyBlock {
			return ActionBlock, reason, StatusComplete
		}
		return ActionWarn, reason, StatusComplete
	case truncated:
		return ActionWarn, "scan stopped at a size cap, results are incomplete", StatusPartial
	}
	return ActionAllow, "", StatusComplete
}

// Describe adds the top findings' rule ids and locations (never values) to a reason.
func Describe(reason string, f []Finding) string {
	var parts []string
	for _, x := range f {
		if x.Allowed {
			continue
		}
		loc := x.Var
		if x.Path != "" {
			loc = fmt.Sprintf("%s:%d", x.Path, x.Line)
		}
		parts = append(parts, x.RuleID+" at "+loc)
		if len(parts) == 3 {
			break
		}
	}
	if len(parts) == 0 {
		return reason
	}
	return reason + ": " + strings.Join(parts, ", ")
}

// Policy returns the app's policy, defaulting to warn.
func (s *Service) Policy(ctx context.Context, app string) (Policy, error) {
	return s.store.GetPolicy(ctx, app)
}

// SetPolicy stores the app's policy.
func (s *Service) SetPolicy(ctx context.Context, app, policy string) (Policy, error) {
	p, err := ParsePolicy(policy)
	if err != nil {
		return "", err
	}
	return p, s.store.SetPolicy(ctx, app, p)
}

// AllowList returns the app's allow-list.
func (s *Service) AllowList(ctx context.Context, app string) ([]AllowEntry, error) {
	return s.store.ListAllow(ctx, app)
}

// Allow adds a fingerprint to the allow-list; who and why are required.
func (s *Service) Allow(ctx context.Context, app, fingerprint, reason, actor string) (AllowEntry, error) {
	fingerprint, reason = strings.TrimSpace(fingerprint), strings.TrimSpace(reason)
	if len(fingerprint) != fingerprintLength || strings.Trim(fingerprint, "0123456789abcdef") != "" {
		return AllowEntry{}, fmt.Errorf("%w: fingerprint must be %d hex characters", ErrInvalid, fingerprintLength)
	}
	if reason == "" {
		return AllowEntry{}, fmt.Errorf("%w: a reason is required", ErrInvalid)
	}
	e := AllowEntry{App: app, Fingerprint: fingerprint, Reason: reason, Actor: actor, CreatedAt: s.now().UTC()}
	return e, s.store.AddAllow(ctx, e)
}

// Unallow removes an allow-list entry and reports whether it existed.
func (s *Service) Unallow(ctx context.Context, app, fingerprint string) (bool, error) {
	return s.store.RemoveAllow(ctx, app, fingerprint)
}

// Run returns the stored scan of a deploy attempt.
func (s *Service) Run(ctx context.Context, attemptID string) (Run, error) {
	return s.store.GetRun(ctx, attemptID)
}

// ScanEnvNow scans an env set without persisting, with the allow-list applied.
func (s *Service) ScanEnvNow(ctx context.Context, app string, env map[string]string) (Result, error) {
	res := ScanEnv(env, s.lim)
	allow, err := s.store.ListAllow(ctx, app)
	if err != nil {
		return Result{}, err
	}
	applyAllow(res.Findings, allow)
	return res, nil
}

// DeleteApp removes an app's policy, allow-list and runs.
func (s *Service) DeleteApp(ctx context.Context, app string) error {
	return s.store.DeleteApp(ctx, app)
}
