package runtimesettings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ErrUnknownKey is returned for a key that is not registered.
var ErrUnknownKey = errors.New("runtimesettings: unknown setting")

const onboardingKey = "onboarding.reviewed"

// Store persists saved values; *store.DB satisfies it.
type Store interface {
	ListRuntimeSettings(ctx context.Context) ([]store.RuntimeSettingRow, error)
	SetRuntimeSetting(ctx context.Context, key, value, actor string, now time.Time) error
	DeleteRuntimeSetting(ctx context.Context, key string) error
}

// Setting is the wire shape of one resolved setting.
type Setting struct {
	Key            string          `json:"key"`
	Category       string          `json:"category"`
	Kind           string          `json:"kind"`
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	Default        json.RawMessage `json:"default"`
	Value          json.RawMessage `json:"value"`
	Source         string          `json:"source"`
	Env            string          `json:"env"`
	Apply          string          `json:"apply"`
	Risk           string          `json:"risk"`
	Onboarding     bool            `json:"onboarding"`
	PendingRestart bool            `json:"pending_restart"`
	UpdatedAt      *time.Time      `json:"updated_at"`
	UpdatedBy      string          `json:"updated_by"`
}

// Service resolves settings. A nil *Service resolves env then default only,
// so callers and tests without a database still get the secure defaults.
type Service struct {
	store  Store
	lookup func(string) (string, bool)
	defs   []Definition
	index  map[string]Definition

	mu     sync.RWMutex
	saved  map[string]store.RuntimeSettingRow
	loaded bool
	boot   map[string]string
}

// New builds a Service over the global registry. lookup defaults to os.LookupEnv.
func New(s Store, lookup func(string) (string, bool)) *Service {
	return NewWithDefinitions(s, lookup, Definitions())
}

// NewWithDefinitions builds a Service over an explicit definition list.
func NewWithDefinitions(s Store, lookup func(string) (string, bool), defs []Definition) *Service {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	idx := make(map[string]Definition, len(defs))
	for _, d := range defs {
		idx[d.Key] = d
	}
	return &Service{store: s, lookup: lookup, defs: defs, index: idx, boot: map[string]string{}}
}

type resolved struct {
	def       Definition
	canonical string
	source    string
	row       *store.RuntimeSettingRow
}

func (s *Service) definition(key string) (Definition, bool) {
	if s == nil {
		for _, d := range Definitions() {
			if d.Key == key {
				return d, true
			}
		}
		return Definition{}, false
	}
	d, ok := s.index[key]
	return d, ok
}

func (s *Service) load(ctx context.Context) (map[string]store.RuntimeSettingRow, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	s.mu.RLock()
	if s.loaded {
		m := s.saved
		s.mu.RUnlock()
		return m, nil
	}
	s.mu.RUnlock()
	rows, err := s.store.ListRuntimeSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("runtimesettings: load saved values: %w", err)
	}
	m := make(map[string]store.RuntimeSettingRow, len(rows))
	for _, r := range rows {
		m[r.Key] = r
	}
	s.mu.Lock()
	s.saved, s.loaded = m, true
	s.mu.Unlock()
	return m, nil
}

func (s *Service) invalidate() {
	s.mu.Lock()
	s.loaded, s.saved = false, nil
	s.mu.Unlock()
}

func (s *Service) lookupEnv(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	if s == nil {
		return os.LookupEnv(name)
	}
	return s.lookup(name)
}

// resolve applies saved > env > default. A saved or env value that no longer
// validates is skipped so a bad value never disables a protection by accident.
func (s *Service) resolve(ctx context.Context, d Definition) (resolved, error) {
	saved, err := s.load(ctx)
	if row, ok := saved[d.Key]; ok {
		if c, perr := canonicalFromString(d, row.Value); perr == nil {
			return resolved{def: d, canonical: c, source: SourceSaved, row: &row}, nil
		}
	}
	if v, ok := s.lookupEnv(d.Env); ok && v != "" {
		if c, perr := canonicalFromString(d, v); perr == nil {
			return resolved{def: d, canonical: c, source: SourceEnv}, err
		}
	}
	c, _ := canonicalFromString(d, d.Default)
	return resolved{def: d, canonical: c, source: SourceDefault}, err
}

func (s *Service) build(r resolved) Setting {
	d := r.def
	def, _ := canonicalFromString(d, d.Default)
	out := Setting{
		Key: d.Key, Category: string(d.Category), Kind: string(d.Kind), Title: d.Title, Description: d.Description,
		Default: typed(d, def), Value: typed(d, r.canonical), Source: r.source, Env: d.Env,
		Apply: string(d.Apply), Risk: string(d.Risk), Onboarding: d.Onboarding,
	}
	if r.row != nil {
		t := r.row.UpdatedAt
		out.UpdatedAt, out.UpdatedBy = &t, r.row.Actor
	}
	if s != nil && d.Apply == ApplyRestart {
		s.mu.RLock()
		bootVal, ok := s.boot[d.Key]
		s.mu.RUnlock()
		out.PendingRestart = ok && bootVal != r.canonical
	}
	return out
}

// Get returns one resolved setting.
func (s *Service) Get(ctx context.Context, key string) (Setting, error) {
	d, ok := s.definition(key)
	if !ok {
		return Setting{}, ErrUnknownKey
	}
	r, err := s.resolve(ctx, d)
	if err != nil {
		return Setting{}, err
	}
	return s.build(r), nil
}

// All returns every registered setting in registration order.
func (s *Service) All(ctx context.Context) ([]Setting, error) {
	defs := Definitions()
	if s != nil {
		defs = s.defs
	}
	out := make([]Setting, 0, len(defs))
	for _, d := range defs {
		r, err := s.resolve(ctx, d)
		if err != nil {
			return nil, err
		}
		out = append(out, s.build(r))
	}
	return out, nil
}

// Onboarding returns the settings flagged for the first-run review.
func (s *Service) Onboarding(ctx context.Context) ([]Setting, error) {
	all, err := s.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Setting, 0, len(all))
	for _, st := range all {
		if st.Onboarding {
			out = append(out, st)
		}
	}
	return out, nil
}

// Set validates raw and saves it. The returned error is a *ValidationError for a bad value.
func (s *Service) Set(ctx context.Context, key string, raw json.RawMessage, actor string) (Setting, error) {
	d, ok := s.definition(key)
	if !ok {
		return Setting{}, ErrUnknownKey
	}
	if s == nil || s.store == nil {
		return Setting{}, errors.New("runtimesettings: storage is not configured")
	}
	c, err := ParseJSON(d, raw)
	if err != nil {
		return Setting{}, err
	}
	if err := s.store.SetRuntimeSetting(ctx, key, c, actor, time.Now()); err != nil {
		return Setting{}, fmt.Errorf("runtimesettings: save %s: %w", key, err)
	}
	s.invalidate()
	return s.Get(ctx, key)
}

// Reset deletes the saved value so env or the default applies again.
func (s *Service) Reset(ctx context.Context, key string) (Setting, error) {
	if _, ok := s.definition(key); !ok {
		return Setting{}, ErrUnknownKey
	}
	if s == nil || s.store == nil {
		return Setting{}, errors.New("runtimesettings: storage is not configured")
	}
	if err := s.store.DeleteRuntimeSetting(ctx, key); err != nil {
		return Setting{}, fmt.Errorf("runtimesettings: reset %s: %w", key, err)
	}
	s.invalidate()
	return s.Get(ctx, key)
}

// WeakensTo reports whether saving raw would make a weakens_security setting
// weaker than it is now: a bool turned off, an allowlist cleared or widened,
// or a duration shortened.
func (s *Service) WeakensTo(ctx context.Context, key string, raw json.RawMessage) bool {
	d, ok := s.definition(key)
	if !ok || d.Risk != RiskWeakensSecurity {
		return false
	}
	next, err := ParseJSON(d, raw)
	if err != nil {
		return false
	}
	cur, _ := s.resolve(ctx, d)
	switch d.Kind {
	case KindBool:
		return next == "false"
	case KindDuration:
		return durationOf(next) < durationOf(cur.canonical)
	case KindStringList:
		old := listOf(cur.canonical)
		if len(old) == 0 {
			return false
		}
		have := map[string]bool{}
		for _, o := range old {
			have[o] = true
		}
		items := listOf(next)
		if len(items) == 0 {
			return true
		}
		for _, it := range items {
			if !have[it] {
				return true
			}
		}
	}
	return false
}

// CaptureBoot records the effective value of every restart-kind setting; the
// accessors keep returning it and pending_restart compares against it.
func (s *Service) CaptureBoot(ctx context.Context) error {
	if s == nil {
		return nil
	}
	for _, d := range s.defs {
		if d.Apply != ApplyRestart {
			continue
		}
		r, err := s.resolve(ctx, d)
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.boot[d.Key] = r.canonical
		s.mu.Unlock()
	}
	return nil
}

// AnyPendingRestart reports whether a restart-kind setting differs from its boot value.
func (s *Service) AnyPendingRestart(ctx context.Context) (bool, error) {
	all, err := s.All(ctx)
	if err != nil {
		return false, err
	}
	for _, st := range all {
		if st.PendingRestart {
			return true, nil
		}
	}
	return false, nil
}

// Reviewed reports whether the first-run security review was acknowledged.
func (s *Service) Reviewed(ctx context.Context) (bool, error) {
	saved, err := s.load(ctx)
	if err != nil {
		return false, err
	}
	_, ok := saved[onboardingKey]
	return ok, nil
}

// AckReviewed marks the first-run security review as done.
func (s *Service) AckReviewed(ctx context.Context, actor string) error {
	if s == nil || s.store == nil {
		return errors.New("runtimesettings: storage is not configured")
	}
	if err := s.store.SetRuntimeSetting(ctx, onboardingKey, "true", actor, time.Now()); err != nil {
		return fmt.Errorf("runtimesettings: ack review: %w", err)
	}
	s.invalidate()
	return nil
}

// value resolves a key to its canonical form; restart-kind settings return the boot value.
func (s *Service) value(ctx context.Context, key string) string {
	d, ok := s.definition(key)
	if !ok {
		return ""
	}
	if s != nil && d.Apply == ApplyRestart {
		s.mu.RLock()
		v, have := s.boot[key]
		s.mu.RUnlock()
		if have {
			return v
		}
	}
	r, _ := s.resolve(ctx, d)
	return r.canonical
}

// Bool returns a bool setting; an unknown key or a non-bool kind yields false.
func (s *Service) Bool(ctx context.Context, key string) bool {
	return s.value(ctx, key) == "true"
}

// Duration returns a duration setting.
func (s *Service) Duration(ctx context.Context, key string) time.Duration {
	return durationOf(s.value(ctx, key))
}

// List returns a string_list setting.
func (s *Service) List(ctx context.Context, key string) []string {
	return listOf(s.value(ctx, key))
}

// Int returns an int setting.
func (s *Service) Int(ctx context.Context, key string) int {
	var n int
	_, _ = fmt.Sscanf(s.value(ctx, key), "%d", &n)
	return n
}
