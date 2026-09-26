package models

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Environment variables that tune virtual keys.
const (
	envKeyGrace    = "APP_MODEL_KEY_ROTATION_GRACE"
	envKeyMaxGrace = "APP_MODEL_KEY_MAX_GRACE"
	envMaxKeys     = "APP_MODEL_MAX_KEYS"
	envMaxLimit    = "APP_MODEL_KEY_MAX_LIMIT"
)

// ErrKeyNotFound is returned when a model has no such key.
var ErrKeyNotFound = errors.New("models: key not found")

// ErrKeyExists is returned when a live key of the model already has the name.
var ErrKeyExists = errors.New("models: a key with this name already exists")

var keyNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,47}$`)

const maxAllowEntries = 50

// KeyLimits are the optional per-key limits. Zero means unlimited and empty
// allow lists mean anything the engine allowlist forwards.
type KeyLimits struct {
	RPM         int
	TPM         int
	MaxParallel int
	AllowPaths  []string
	AllowModels []string
}

// CreateKeyInput is a request for a new virtual key.
type CreateKeyInput struct {
	Name      string
	ExpiresAt *time.Time
	Limits    KeyLimits
}

// KeyStatus describes where a key is in its life.
type KeyStatus string

// Key statuses.
const (
	KeyActive   KeyStatus = "active"
	KeyRotating KeyStatus = "rotating"
	KeyExpired  KeyStatus = "expired"
	KeyRevoked  KeyStatus = "revoked"
)

// KeyView is a key without its hash, with its derived status.
type KeyView struct {
	store.ModelKey
	Status   KeyStatus
	InFlight int
}

// CreatedKey is a new key; Plaintext is shown once and never stored.
type CreatedKey struct {
	KeyView
	Plaintext string
}

func keyStatus(k store.ModelKey, now time.Time) KeyStatus {
	switch {
	case k.RevokedAt != nil && !now.Before(*k.RevokedAt):
		return KeyRevoked
	case k.ExpiresAt != nil && !now.Before(*k.ExpiresAt):
		return KeyExpired
	case k.ReplacedBy != "":
		return KeyRotating
	}
	return KeyActive
}

// SetKeyChangeHook registers fn to run after any key is created, revoked or
// rotated, so the gateway can drop its cached key list.
func (s *Service) SetKeyChangeHook(fn func()) { s.onKeysChanged = fn }

// SetLiveStats registers a source of running request counts per key id.
func (s *Service) SetLiveStats(fn func() map[string]int) { s.liveStats = fn }

func (s *Service) keysChanged() {
	if s.onKeysChanged != nil {
		s.onKeysChanged()
	}
}

func (s *Service) keyView(k store.ModelKey, now time.Time, live map[string]int) KeyView {
	return KeyView{ModelKey: k, Status: keyStatus(k, now), InFlight: live[k.ID]}
}

func (s *Service) live() map[string]int {
	if s.liveStats == nil {
		return nil
	}
	return s.liveStats()
}

// ListKeys returns every key of a model, revoked ones included.
func (s *Service) ListKeys(ctx context.Context, model string) ([]KeyView, error) {
	if _, err := s.store.GetModel(ctx, model); err != nil {
		return nil, err
	}
	keys, err := s.store.ListModelKeys(ctx, model)
	if err != nil {
		return nil, fmt.Errorf("models: list keys of %q: %w", model, err)
	}
	now, live := time.Now(), s.live()
	out := make([]KeyView, len(keys))
	for i, k := range keys {
		out[i] = s.keyView(k, now, live)
	}
	return out, nil
}

func validateLimits(engine string, l KeyLimits) error {
	maxLimit := envInt(envMaxLimit, 10_000_000)
	for name, v := range map[string]int{"rpm": l.RPM, "tpm": l.TPM, "max_parallel": l.MaxParallel} {
		if v < 0 || (maxLimit > 0 && v > maxLimit) {
			return fmt.Errorf("%w: %s must be between 0 and %d", ErrInvalid, name, maxLimit)
		}
	}
	if len(l.AllowPaths) > maxAllowEntries || len(l.AllowModels) > maxAllowEntries {
		return fmt.Errorf("%w: at most %d allow list entries", ErrInvalid, maxAllowEntries)
	}
	if bad, ok := ValidateAllowPaths(engine, l.AllowPaths); !ok {
		return fmt.Errorf("%w: path %q is not served by the %s gateway", ErrInvalid, bad, engine)
	}
	for _, m := range l.AllowModels {
		if strings.TrimSpace(m) == "" || len(m) > 200 {
			return fmt.Errorf("%w: model allow list entries must be 1 to 200 characters", ErrInvalid)
		}
	}
	return nil
}

func (s *Service) checkKeyCapacity(ctx context.Context, model string) error {
	limit := envInt(envMaxKeys, 50)
	if limit <= 0 {
		return nil
	}
	keys, err := s.store.ListModelKeys(ctx, model)
	if err != nil {
		return fmt.Errorf("models: list keys of %q: %w", model, err)
	}
	live := 0
	for _, k := range keys {
		if k.RevokedAt == nil {
			live++
		}
	}
	if live >= limit {
		return fmt.Errorf("%w: a model may have at most %d keys", ErrInvalid, limit)
	}
	return nil
}

// CreateKey adds a named key to a model and returns its plaintext once.
func (s *Service) CreateKey(ctx context.Context, model string, in CreateKeyInput) (CreatedKey, error) {
	m, err := s.store.GetModel(ctx, model)
	if err != nil {
		return CreatedKey{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	if !keyNameRe.MatchString(in.Name) {
		return CreatedKey{}, fmt.Errorf("%w: key name must be 1 to 48 letters, digits, dots, dashes or underscores", ErrInvalid)
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now()) {
		return CreatedKey{}, fmt.Errorf("%w: expiry must be in the future", ErrInvalid)
	}
	if err := validateLimits(m.Engine, in.Limits); err != nil {
		return CreatedKey{}, err
	}
	if err := s.checkKeyCapacity(ctx, model); err != nil {
		return CreatedKey{}, err
	}
	return s.insertKey(ctx, model, in.Name, in.ExpiresAt, in.Limits, nil, nil)
}

// insertKey creates the key, or with old set rotates old into it.
func (s *Service) insertKey(ctx context.Context, model, name string, expires *time.Time, l KeyLimits, old *store.ModelKey, graceUntil *time.Time) (CreatedKey, error) {
	plain, hash, prefix, err := NewAPIKey()
	if err != nil {
		return CreatedKey{}, err
	}
	id, err := NewKeyID()
	if err != nil {
		return CreatedKey{}, err
	}
	now := time.Now().UTC()
	k := store.ModelKey{ID: id, ModelName: model, Name: name, KeyHash: hash, KeyPrefix: prefix, RPM: l.RPM, TPM: l.TPM,
		MaxParallel: l.MaxParallel, AllowPaths: l.AllowPaths, AllowModels: l.AllowModels, CreatedAt: now, ExpiresAt: expires}
	if old == nil {
		err = s.store.CreateModelKey(ctx, k)
	} else {
		err = s.store.RotateModelKey(ctx, model, old.ID, k, graceUntil, now)
	}
	switch {
	case errors.Is(err, store.ErrModelKeyExists):
		return CreatedKey{}, ErrKeyExists
	case errors.Is(err, store.ErrModelKeyNotFound):
		return CreatedKey{}, ErrKeyNotFound
	case err != nil:
		return CreatedKey{}, fmt.Errorf("models: save key %q of %q: %w", name, model, err)
	}
	s.keysChanged()
	return CreatedKey{KeyView: s.keyView(k, now, nil), Plaintext: plain}, nil
}

// RevokeKey stops a key from working at once.
func (s *Service) RevokeKey(ctx context.Context, model, id string) error {
	if err := s.store.RevokeModelKey(ctx, model, id, time.Now().UTC()); err != nil {
		if errors.Is(err, store.ErrModelKeyNotFound) {
			return ErrKeyNotFound
		}
		return fmt.Errorf("models: revoke key %q of %q: %w", id, model, err)
	}
	s.keysChanged()
	return nil
}

// KeyRotationGrace returns the grace window used when a rotation names none.
func KeyRotationGrace() time.Duration { return envDuration(envKeyGrace, time.Hour) }

// RotateKeyByID issues a replacement for a key with the same name, limits
// and expiry. The old key keeps working for grace (KeyRotationGrace when
// nil, no grace when zero) and is then dead.
func (s *Service) RotateKeyByID(ctx context.Context, model, id string, grace *time.Duration) (CreatedKey, error) {
	old, err := s.store.GetModelKey(ctx, model, id)
	if err != nil {
		if errors.Is(err, store.ErrModelKeyNotFound) {
			return CreatedKey{}, ErrKeyNotFound
		}
		return CreatedKey{}, fmt.Errorf("models: get key %q of %q: %w", id, model, err)
	}
	now := time.Now().UTC()
	if st := keyStatus(*old, now); st != KeyActive {
		return CreatedKey{}, fmt.Errorf("%w: only an active key can be rotated, this one is %s", ErrInvalid, st)
	}
	g := KeyRotationGrace()
	if grace != nil {
		g = *grace
	}
	if maxGrace := envDuration(envKeyMaxGrace, 7*24*time.Hour); g < 0 || (maxGrace > 0 && g > maxGrace) {
		return CreatedKey{}, fmt.Errorf("%w: grace must be between 0 and %s", ErrInvalid, maxGrace)
	}
	var until *time.Time
	if g > 0 {
		t := now.Add(g)
		until = &t
	}
	l := KeyLimits{RPM: old.RPM, TPM: old.TPM, MaxParallel: old.MaxParallel, AllowPaths: old.AllowPaths, AllowModels: old.AllowModels}
	return s.insertKey(ctx, model, old.Name, old.ExpiresAt, l, old, until)
}
