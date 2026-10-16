package fixit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"
)

// DefaultMaxPerHour caps mutating applies per fix per hour.
const DefaultMaxPerHour = 3

const attemptHistory = 5

var tiers = []Kind{KindAuto, KindGuided, KindManual}

// Engine is the registry plus the apply state machine.
type Engine struct {
	store      Store
	fixes      map[string]Fix
	order      []string
	now        func() time.Time
	maxPerHour int
}

// EngineOption configures an Engine.
type EngineOption func(*Engine)

// WithClock overrides the time source, for tests.
func WithClock(now func() time.Time) EngineOption { return func(e *Engine) { e.now = now } }

// WithMaxPerHour overrides the per-fix hourly apply cap; zero keeps the default.
func WithMaxPerHour(n int) EngineOption {
	return func(e *Engine) {
		if n > 0 {
			e.maxPerHour = n
		}
	}
}

// NewEngine builds an engine over an attempt store.
func NewEngine(store Store, opts ...EngineOption) *Engine {
	e := &Engine{store: store, fixes: map[string]Fix{}, now: time.Now, maxPerHour: DefaultMaxPerHour}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Register adds a fix. An auto fix without Run is a programming error.
func (e *Engine) Register(f Fix) error {
	if f.ID == "" || f.Detect == nil {
		return fmt.Errorf("fixit: fix needs an id and Detect")
	}
	if f.Kind == KindAuto && f.Run == nil {
		return fmt.Errorf("fixit: auto fix %s needs Run", f.ID)
	}
	if _, dup := e.fixes[f.ID]; dup {
		return fmt.Errorf("fixit: duplicate fix %s", f.ID)
	}
	e.fixes[f.ID] = f
	e.order = append(e.order, f.ID)
	return nil
}

// Fingerprint identifies one failing check result.
func Fingerprint(checkID string, a Applicability) string {
	sig := a.Signature
	if sig == "" {
		sig = a.Detail
	}
	sum := sha256.Sum256([]byte(checkID + "\x00" + sig))
	return hex.EncodeToString(sum[:8])
}

func tierIndex(k Kind) int {
	for i, t := range tiers {
		if t == k {
			return i
		}
	}
	return len(tiers) - 1
}

// plan decides the effective tier from the host finding and attempt memory.
// Attempts are newest first; only unbroken same-fingerprint failures demote.
func plan(f Fix, a Applicability, attempts []Attempt, fp string) (Kind, int) {
	base := f.Kind
	if a.Kind != "" && tierIndex(a.Kind) > tierIndex(base) {
		base = a.Kind
	}
	failed := 0
	for _, at := range attempts {
		if at.Fingerprint != fp || (at.Outcome != OutcomeStillFailing && at.Outcome != OutcomeCannotFixHere) {
			break
		}
		failed++
	}
	idx := tierIndex(base) + failed
	if idx >= len(tiers) {
		idx = len(tiers) - 1
	}
	eff := tiers[idx]
	if eff == KindGuided && a.Command == "" {
		eff = KindManual
	}
	return eff, failed
}

func (e *Engine) view(ctx context.Context, f Fix, a Applicability) (View, error) {
	attempts, err := e.store.RecentAttempts(ctx, f.ID, f.Subject, attemptHistory)
	if err != nil {
		return View{}, fmt.Errorf("fixit: load attempts: %w", err)
	}
	fp := Fingerprint(f.CheckID, a)
	eff, failed := plan(f, a, attempts, fp)
	v := View{
		ID: f.ID, CheckID: f.CheckID, Subject: f.Subject, Title: f.Title, Why: f.Why, Risk: f.Risk,
		Reversible: f.Reversible, Needs: f.Needs, BaseKind: f.Kind, Kind: eff, Failing: a.Failing,
		Detail: a.Detail, Reason: a.Reason, Changes: a.Changes, Command: a.Command, Param: a.Param,
		Attempts: attempts, State: StateOffer,
	}
	if v.Changes == nil {
		v.Changes = []Change{}
	}
	if v.Attempts == nil {
		v.Attempts = []Attempt{}
	}
	if len(attempts) > 0 {
		last := attempts[0]
		v.LastTried = &last
	}
	switch {
	case !a.Failing:
		v.State = StateHealthy
	case failed > 0:
		v.State = StateDemoted
		if v.LastTried != nil {
			v.Blocked = fmt.Sprintf("tried at %s, still blocked because %s", v.LastTried.At.UTC().Format(time.RFC3339), v.LastTried.Detail)
		}
	}
	if a.Failing && eff == KindAuto {
		n, err := e.store.CountAttemptsSince(ctx, f.ID, e.now().Add(-time.Hour))
		if err != nil {
			return View{}, fmt.Errorf("fixit: count attempts: %w", err)
		}
		if n >= e.maxPerHour {
			v.State = StateRateLimited
			v.Blocked = fmt.Sprintf("applied %d times in the last hour, waiting before trying again", n)
		}
	}
	return v, nil
}

func (e *Engine) get(id string) (Fix, error) {
	f, ok := e.fixes[id]
	if !ok {
		return Fix{}, fmt.Errorf("%w: %s", ErrUnknownFix, id)
	}
	return f, nil
}

// List returns fixes for a subject ("" for all). Healthy fixes are left out
// unless includeHealthy is set. Listing re-runs each check but never mutates.
func (e *Engine) List(ctx context.Context, subject string, includeHealthy bool) ([]View, error) {
	out := []View{}
	ids := append([]string(nil), e.order...)
	sort.Strings(ids)
	for _, id := range ids {
		f := e.fixes[id]
		if subject != "" && f.Subject != subject {
			continue
		}
		a, err := f.Detect(ctx)
		if err != nil {
			continue
		}
		v, err := e.view(ctx, f, a)
		if err != nil {
			return nil, err
		}
		if v.State == StateHealthy && !includeHealthy {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

// Preview returns one fix's current plan without changing anything.
func (e *Engine) Preview(ctx context.Context, id string) (View, error) {
	f, err := e.get(id)
	if err != nil {
		return View{}, err
	}
	a, err := f.Detect(ctx)
	if err != nil {
		return View{}, fmt.Errorf("fixit: detect %s: %w", id, err)
	}
	return e.view(ctx, f, a)
}

// Apply runs the fix (auto tier) or re-checks it (guided and manual tiers),
// then re-verifies by re-running the originating check. It never runs a
// mutation the demotion or rate limit has withdrawn.
func (e *Engine) Apply(ctx context.Context, id, actor string, params map[string]string) (Result, error) {
	f, err := e.get(id)
	if err != nil {
		return Result{}, err
	}
	a, err := f.Detect(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("fixit: detect %s: %w", id, err)
	}
	v, err := e.view(ctx, f, a)
	if err != nil {
		return Result{}, err
	}
	res := Result{FixID: id, View: v}
	if !a.Failing {
		res.Outcome, res.Detail = OutcomeSkipped, "already healthy, nothing to do"
		return res, nil
	}
	if v.State == StateRateLimited {
		res.Outcome, res.Detail = OutcomeSkipped, v.Blocked
		return res, nil
	}
	fp := Fingerprint(f.CheckID, a)
	var runErr error
	if v.Kind == KindAuto {
		runErr = f.Run(ctx, Input{Actor: actor, Params: params})
	}
	after, derr := f.Detect(ctx)
	outcome, detail := OutcomeFixed, "check passes"
	switch {
	case runErr != nil && errors.Is(runErr, ErrCannotFixHere):
		outcome, detail = OutcomeCannotFixHere, runErr.Error()
	case runErr != nil:
		outcome, detail = OutcomeStillFailing, runErr.Error()
	case derr != nil:
		outcome, detail = OutcomeStillFailing, "could not re-check: "+derr.Error()
	case after.Failing:
		outcome, detail = OutcomeStillFailing, after.Detail
		fp = Fingerprint(f.CheckID, after)
	}
	rec := Attempt{FixID: id, Subject: f.Subject, At: e.now().UTC(), Actor: actor, Outcome: outcome, Detail: detail, Fingerprint: fp}
	if err := e.store.RecordAttempt(ctx, rec); err != nil {
		return Result{}, fmt.Errorf("fixit: record attempt: %w", err)
	}
	res.Outcome, res.Detail = outcome, detail
	nv, err := e.view(ctx, f, after)
	if err != nil {
		return Result{}, err
	}
	res.View = nv
	return res, nil
}
