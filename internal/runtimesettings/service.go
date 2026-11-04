package runtimesettings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
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
	Sensitive      bool            `json:"sensitive"`
	AllowedValues  []int           `json:"allowed_values,omitempty"`
	AllowedStrings []string        `json:"allowed_strings,omitempty"`
	PendingRestart bool            `json:"pending_restart"`
	// WeakerThanDefault is true when a weakens_security setting now holds a weaker value than its default.
	WeakerThanDefault bool       `json:"weaker_than_default"`
	UpdatedAt         *time.Time `json:"updated_at"`
	UpdatedBy         string     `json:"updated_by"`
}

// Service resolves settings. A nil *Service resolves env then default only,
// so callers and tests without a database still get the secure defaults.
type Service struct {
	store  Store
	lookup func(string) (string, bool)
	defs   []Definition
	index  map[string]Definition
	// live follows the global registry, so a setting registered after New (a cleanup target) still resolves.
	live bool

	mu     sync.RWMutex
	saved  map[string]store.RuntimeSettingRow
	loaded bool
	boot   map[string]string
	// lastGood and retryAt serve the previous rows briefly after a failed load, so a database error is not a query per request.
	lastGood map[string]store.RuntimeSettingRow
	retryAt  time.Time
}

// loadRetryInterval is how long a failed load is not retried; APP_RUNTIME_SETTINGS_LOAD_RETRY overrides the 2s default.
func loadRetryInterval() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("APP_RUNTIME_SETTINGS_LOAD_RETRY")); err == nil && d > 0 {
		return d
	}
	return 2 * time.Second
}

// New builds a Service over the global registry. lookup defaults to os.LookupEnv.
func New(s Store, lookup func(string) (string, bool)) *Service {
	svc := NewWithDefinitions(s, lookup, Definitions())
	svc.live = true
	return svc
}

// snapshot returns the definitions and index, refreshing them first when the global registry grew.
func (s *Service) snapshot() ([]Definition, map[string]Definition) {
	if s.live && registryLen() != len(s.defsLocked()) {
		defs := Definitions()
		idx := make(map[string]Definition, len(defs))
		for _, d := range defs {
			idx[d.Key] = d
		}
		s.mu.Lock()
		s.defs, s.index = defs, idx
		s.mu.Unlock()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.defs, s.index
}

func (s *Service) defsLocked() []Definition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.defs
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
	// invalid names a saved or env value that failed validation on a FailClosed setting.
	invalid string
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
	_, idx := s.snapshot()
	d, ok := idx[key]
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
	stale, retryAt := s.lastGood, s.retryAt
	s.mu.RUnlock()
	if time.Now().Before(retryAt) {
		if stale != nil {
			return stale, nil
		}
		return nil, errors.New("runtimesettings: load saved values: database unavailable, retrying shortly")
	}
	rows, err := s.store.ListRuntimeSettings(ctx)
	if err != nil {
		s.mu.Lock()
		s.retryAt = time.Now().Add(loadRetryInterval())
		s.mu.Unlock()
		if stale != nil {
			return stale, nil
		}
		return nil, fmt.Errorf("runtimesettings: load saved values: %w", err)
	}
	m := make(map[string]store.RuntimeSettingRow, len(rows))
	for _, r := range rows {
		m[r.Key] = r
	}
	s.mu.Lock()
	s.saved, s.loaded, s.lastGood, s.retryAt = m, true, m, time.Time{}
	s.mu.Unlock()
	return m, nil
}

func (s *Service) invalidate() {
	s.mu.Lock()
	s.loaded, s.saved, s.retryAt = false, nil, time.Time{}
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
	invalid := ""
	if row, ok := saved[d.Key]; ok {
		if c, perr := canonicalFromString(d, row.Value); perr == nil {
			return resolved{def: d, canonical: c, source: SourceSaved, row: &row}, nil
		}
		invalid = fmt.Sprintf("the saved setting %s holds an invalid value", d.Key)
	}
	if v, ok := s.lookupEnv(d.Env); ok && v != "" {
		if c, perr := canonicalEnv(d, v); perr == nil && (invalid == "" || !d.FailClosed) {
			return resolved{def: d, canonical: c, source: SourceEnv}, err
		}
		if invalid == "" {
			invalid = fmt.Sprintf("the environment variable %s holds an invalid value", d.Env)
		}
		warnInvalidEnv(d)
	}
	c, _ := canonicalFromString(d, d.Default)
	r := resolved{def: d, canonical: c, source: SourceDefault, invalid: invalid}
	if !d.FailClosed {
		r.invalid = ""
	}
	return r, err
}

// maskSensitive hides a credential-like value but still tells the UI whether one is set.
func maskSensitive(v json.RawMessage) json.RawMessage {
	switch strings.TrimSpace(string(v)) {
	case "", `""`, "null", "[]", "false", "0":
		return v
	}
	return json.RawMessage(`"***"`)
}

func (s *Service) build(r resolved) Setting {
	d := r.def
	def, _ := canonicalFromString(d, d.Default)
	out := Setting{
		Key: d.Key, Category: string(d.Category), Kind: string(d.Kind), Title: d.Title, Description: d.Description,
		Default: typed(d, def), Value: typed(d, r.canonical), Source: r.source, Env: d.Env,
		Apply: string(d.Apply), Risk: string(d.Risk), Onboarding: d.Onboarding, Sensitive: d.Sensitive,
		AllowedValues: d.AllowedInts, AllowedStrings: d.AllowedStrings,
	}
	if d.Risk == RiskWeakensSecurity && r.canonical != def {
		out.WeakerThanDefault = weaker(d, def, r.canonical)
	}
	if d.Sensitive {
		out.Default, out.Value = maskSensitive(out.Default), maskSensitive(out.Value)
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
		defs, _ = s.snapshot()
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

// ErrWeakeningNotConfirmed is returned when a write weakens a protection and the caller did not confirm it.
var ErrWeakeningNotConfirmed = errors.New("runtimesettings: change weakens a security protection and was not confirmed")

// ErrSensitiveKey is returned when a caller that may not write credentials targets a sensitive setting.
var ErrSensitiveKey = errors.New("runtimesettings: sensitive setting cannot be written by this caller")

// WriteOptions carries what the caller is allowed to do; the zero value is the strictest.
type WriteOptions struct {
	ConfirmWeakening bool
	RefuseSensitive  bool
}

// ValidateForWrite checks raw against key's type, range, enum and floor rules and the
// caller's permissions, returning the canonical value. Every writer goes through it.
func (s *Service) ValidateForWrite(ctx context.Context, key string, raw json.RawMessage, opts WriteOptions) (string, error) {
	d, ok := s.definition(key)
	if !ok {
		return "", ErrUnknownKey
	}
	if opts.RefuseSensitive && d.Sensitive {
		return "", ErrSensitiveKey
	}
	c, err := ParseJSON(d, raw)
	if err != nil {
		return "", err
	}
	if !opts.ConfirmWeakening && s.WeakensTo(ctx, key, raw) {
		return "", ErrWeakeningNotConfirmed
	}
	return c, nil
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

// WeakensOnReset reports whether deleting the saved value would leave a weaker
// environment seed than the one in force now. Returning to the built-in default
// never needs confirmation, so only an explicit env seed is compared.
func (s *Service) WeakensOnReset(ctx context.Context, key string) bool {
	d, ok := s.definition(key)
	if !ok || d.Risk != RiskWeakensSecurity {
		return false
	}
	cur, _ := s.resolve(ctx, d)
	if cur.source != SourceSaved {
		return false
	}
	v, ok := s.lookupEnv(d.Env)
	if !ok || v == "" {
		return false
	}
	target, err := canonicalEnv(d, v)
	if err != nil {
		return false
	}
	c, err := canonicalFromString(d, target)
	if err != nil {
		return false
	}
	return s.WeakensTo(ctx, key, typed(d, c))
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
	return weaker(d, cur.canonical, next)
}

// weaker reports whether next is a weaker canonical value than base for d.
func weaker(d Definition, base, next string) bool {
	switch d.Kind {
	case KindBool:
		return next == "false"
	case KindDuration:
		return durationOf(next) < durationOf(base)
	case KindString:
		order := d.StrengthOrder
		return slices.Index(order, base) >= 0 && slices.Index(order, next) < slices.Index(order, base)
	case KindInt:
		if d.HigherIsWeaker {
			return intOf(next) > intOf(base)
		}
		return shorterDays(base, next)
	case KindStringList:
		old := listOf(base)
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
	defs, _ := s.snapshot()
	for _, d := range defs {
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

// StringChecked returns a string setting for a security decision. Unlike
// String it errors, naming where the problem came from, when the saved
// settings cannot be read or a FailClosed setting holds an invalid value.
func (s *Service) StringChecked(ctx context.Context, key string) (string, error) {
	d, ok := s.definition(key)
	if !ok {
		return "", fmt.Errorf("runtimesettings: unknown setting %q", key)
	}
	r, err := s.resolve(ctx, d)
	if d.FailClosed && err != nil {
		return "", fmt.Errorf("%s: saved settings could not be read: %w", key, err)
	}
	if r.invalid != "" {
		return "", fmt.Errorf("%s: %s; fix it or remove it (allowed: %s)", key, r.invalid, strings.Join(d.AllowedStrings, ", "))
	}
	return r.canonical, nil
}

// Source reports whether a key's value is saved, from the environment, or the default.
func (s *Service) Source(ctx context.Context, key string) string {
	d, ok := s.definition(key)
	if !ok {
		return SourceDefault
	}
	r, _ := s.resolve(ctx, d)
	return r.source
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

// String returns a string setting.
func (s *Service) String(ctx context.Context, key string) string {
	return s.value(ctx, key)
}

// shorterDays reports whether next keeps data for fewer days than cur; 0 means forever.
func shorterDays(cur, next string) bool {
	var c, n int
	_, _ = fmt.Sscanf(cur, "%d", &c)
	_, _ = fmt.Sscanf(next, "%d", &n)
	switch {
	case n == c || n == 0:
		return false
	case c == 0:
		return true
	}
	return n < c
}

func intOf(canonical string) int {
	n, _ := strconv.Atoi(canonical)
	return n
}

// Int returns an int setting.
func (s *Service) Int(ctx context.Context, key string) int {
	var n int
	_, _ = fmt.Sscanf(s.value(ctx, key), "%d", &n)
	return n
}

var warnedInvalidEnv sync.Map

// warnInvalidEnv logs once per setting; the value itself is never logged because it may be a secret.
func warnInvalidEnv(d Definition) {
	if _, seen := warnedInvalidEnv.LoadOrStore(d.Key, struct{}{}); seen {
		return
	}
	slog.Warn("runtime setting ignored an invalid environment value, using the saved or default value", slog.String("key", d.Key), slog.String("env", d.Env))
}
