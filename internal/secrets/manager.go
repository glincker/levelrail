package secrets

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Store is the narrow surface Manager needs from internal/store, so
// tests can fake it without a real database.
type Store interface {
	GetServiceDEK(ctx context.Context, serviceName string) ([]byte, error)
	SaveServiceDEK(ctx context.Context, serviceName string, wrappedDEK []byte) error
	GetSecretValue(ctx context.Context, serviceName, envKey string) ([]byte, error)
	SaveSecretValue(ctx context.Context, serviceName, envKey string, ciphertext []byte) error
	HasSecretValue(ctx context.Context, serviceName, envKey string) (bool, error)
	DeleteServiceSecrets(ctx context.Context, serviceName string) error
	ListSecretKeys(ctx context.Context, serviceName string) ([]store.SecretKeyInfo, error)
	GetSecretKeyLocked(ctx context.Context, serviceName, envKey string) (exists, locked bool, err error)
	SetSecretLocked(ctx context.Context, serviceName, envKey string, locked bool) error
	// RotateServiceDEKs rewraps every stored DEK in one transaction; any
	// rewrap error rolls the whole rotation back.
	RotateServiceDEKs(ctx context.Context, rewrap func(serviceName string, wrapped []byte) ([]byte, error)) error
	GetMasterKeyRotatedAt(ctx context.Context) (rotatedAt time.Time, ok bool, err error)
	CountSecretValuesByPrefix(ctx context.Context, prefix []byte) (total, withPrefix int, err error)
	ListSecretCiphertexts(ctx context.Context, afterService, afterKey string, limit int) ([]store.SecretCiphertext, error)
	// ReplaceSecretCiphertext swaps old for replacement only while the row
	// still holds old, so a concurrent SetValue is never clobbered.
	ReplaceSecretCiphertext(ctx context.Context, serviceName, envKey string, old, replacement []byte) (bool, error)
}

// ScopeServiceSecret is the Binding scope of every value Manager stores.
const ScopeServiceSecret = "service_secret"

// ErrValueNotFound means no secret value has been set for a given
// (service, env key) pair, whether the DEK or the ciphertext is missing.
var ErrValueNotFound = errors.New("secrets: value not found")

// ErrSecretLocked is returned by SetValueGuarded when overwriting an
// existing, locked value without overwriteLocked=true; callers surface
// it as a 409.
var ErrSecretLocked = errors.New("secrets: value is locked")

// ErrLegacyRejected is returned by Resolve for a legacy unbound
// ciphertext when the Manager was built WithRequireBound(true).
var ErrLegacyRejected = errors.New("secrets: legacy unbound ciphertext rejected")

// Manager combines a MasterKey with a Store to provide per-app envelope
// encryption end to end, binding every value to its (service, key) slot.
type Manager struct {
	store Store
	// mu guards mk: RotateMasterKey holds the write lock for the whole
	// rotation so nothing reads or wraps under the old key mid-rotation.
	mu sync.RWMutex
	mk *MasterKey

	logger       *slog.Logger
	requireBound bool
	legacySeen   sync.Map
	rebindMu     sync.Mutex
}

// ManagerOption configures a Manager.
type ManagerOption func(*Manager)

// WithLogger sets the logger Manager uses for legacy-format notices.
func WithLogger(l *slog.Logger) ManagerOption {
	return func(m *Manager) {
		if l != nil {
			m.logger = l
		}
	}
}

// WithRequireBound makes Resolve reject legacy unbound ciphertexts.
func WithRequireBound(require bool) ManagerOption {
	return func(m *Manager) { m.requireBound = require }
}

// NewManager builds a Manager around mk, the control plane's in-memory
// master key.
func NewManager(store Store, mk *MasterKey, opts ...ManagerOption) *Manager {
	m := &Manager{store: store, mk: mk, logger: slog.New(slog.DiscardHandler)}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func slotBinding(serviceName, envKey string) Binding {
	return Binding{Scope: ScopeServiceSecret, Owner: serviceName, Key: envKey}
}

// SetValue encrypts plaintext under serviceName's DEK, generating one on
// first use, and persists only the ciphertext.
func (m *Manager) SetValue(ctx context.Context, serviceName, envKey, plaintext string) error {
	dek, err := m.dekFor(ctx, serviceName)
	if err != nil {
		return err
	}

	ciphertext, err := EncryptValue(dek, slotBinding(serviceName, envKey), plaintext)
	if err != nil {
		return fmt.Errorf("secrets: encrypt value for %q/%q: %w", serviceName, envKey, err)
	}

	if err := m.store.SaveSecretValue(ctx, serviceName, envKey, ciphertext); err != nil {
		return fmt.Errorf("secrets: save value for %q/%q: %w", serviceName, envKey, err)
	}
	return nil
}

// SetValueGuarded is SetValue that refuses, with ErrSecretLocked, to
// overwrite an existing locked value unless overwriteLocked is true.
func (m *Manager) SetValueGuarded(ctx context.Context, serviceName, envKey, plaintext string, overwriteLocked bool) error {
	exists, locked, err := m.store.GetSecretKeyLocked(ctx, serviceName, envKey)
	if err != nil {
		return fmt.Errorf("secrets: check locked state for %q/%q: %w", serviceName, envKey, err)
	}
	if exists && locked && !overwriteLocked {
		return ErrSecretLocked
	}
	return m.SetValue(ctx, serviceName, envKey, plaintext)
}

// ListKeys returns every secret key set for serviceName with its locked
// state, never a value.
func (m *Manager) ListKeys(ctx context.Context, serviceName string) ([]store.SecretKeyInfo, error) {
	keys, err := m.store.ListSecretKeys(ctx, serviceName)
	if err != nil {
		return nil, fmt.Errorf("secrets: list keys for %q: %w", serviceName, err)
	}
	return keys, nil
}

// SetLocked toggles (serviceName, envKey)'s locked flag, reversible in
// either direction. Returns store.ErrSecretValueNotFound if no value has
// been set for that key yet.
func (m *Manager) SetLocked(ctx context.Context, serviceName, envKey string, locked bool) error {
	if err := m.store.SetSecretLocked(ctx, serviceName, envKey, locked); err != nil {
		return fmt.Errorf("secrets: set locked for %q/%q: %w", serviceName, envKey, err)
	}
	return nil
}

// Resolve decrypts and returns the plaintext value for (serviceName,
// envKey), or ErrValueNotFound if none was ever set. A ciphertext bound
// to any other slot fails with ErrBindingMismatch. Callers must never
// persist what this returns.
func (m *Manager) Resolve(ctx context.Context, serviceName, envKey string) (string, error) {
	wrapped, err := m.store.GetServiceDEK(ctx, serviceName)
	if errors.Is(err, store.ErrServiceDEKNotFound) {
		return "", ErrValueNotFound
	}
	if err != nil {
		return "", fmt.Errorf("secrets: get DEK for %q: %w", serviceName, err)
	}

	ciphertext, err := m.store.GetSecretValue(ctx, serviceName, envKey)
	if errors.Is(err, store.ErrSecretValueNotFound) {
		return "", ErrValueNotFound
	}
	if err != nil {
		return "", fmt.Errorf("secrets: get value for %q/%q: %w", serviceName, envKey, err)
	}

	dek, err := m.masterKey().UnwrapDEK(WrappedDEK(wrapped))
	if err != nil {
		return "", fmt.Errorf("secrets: unwrap DEK for %q: %w", serviceName, err)
	}

	plaintext, legacy, err := DecryptValue(dek, slotBinding(serviceName, envKey), ciphertext)
	if err != nil {
		return "", fmt.Errorf("secrets: decrypt value for %q/%q: %w", serviceName, envKey, err)
	}
	if legacy {
		if m.requireBound {
			return "", fmt.Errorf("secrets: decrypt value for %q/%q: %w", serviceName, envKey, ErrLegacyRejected)
		}
		m.noteLegacy(serviceName, envKey)
	}
	return plaintext, nil
}

// noteLegacy logs a legacy read once per slot per process, not per read.
func (m *Manager) noteLegacy(serviceName, envKey string) {
	if _, seen := m.legacySeen.LoadOrStore(serviceName+"\x00"+envKey, struct{}{}); seen {
		return
	}
	m.logger.Debug("secrets: read a legacy unbound ciphertext, run secrets rebind to bind it",
		slog.String("scope", ScopeServiceSecret), slog.String("key", envKey))
}

// Exists reports whether a value has been set for (serviceName, envKey),
// without decrypting it.
func (m *Manager) Exists(ctx context.Context, serviceName, envKey string) (bool, error) {
	ok, err := m.store.HasSecretValue(ctx, serviceName, envKey)
	if err != nil {
		return false, fmt.Errorf("secrets: check value for %q/%q: %w", serviceName, envKey, err)
	}
	return ok, nil
}

// DeleteAll permanently removes every value and the wrapped DEK for
// serviceName, so nothing set under it can ever be decrypted again.
func (m *Manager) DeleteAll(ctx context.Context, serviceName string) error {
	if err := m.store.DeleteServiceSecrets(ctx, serviceName); err != nil {
		return fmt.Errorf("secrets: delete all values for %q: %w", serviceName, err)
	}
	return nil
}

// dekFor returns serviceName's raw DEK, generating and persisting a new
// wrapped one on first use. An existing DEK is never replaced.
func (m *Manager) dekFor(ctx context.Context, serviceName string) ([]byte, error) {
	mk := m.masterKey()

	wrapped, err := m.store.GetServiceDEK(ctx, serviceName)
	if err == nil {
		return mk.UnwrapDEK(WrappedDEK(wrapped))
	}
	if !errors.Is(err, store.ErrServiceDEKNotFound) {
		return nil, fmt.Errorf("secrets: get DEK for %q: %w", serviceName, err)
	}

	raw, newlyWrapped, err := mk.GenerateDEK()
	if err != nil {
		return nil, fmt.Errorf("secrets: generate DEK for %q: %w", serviceName, err)
	}
	if err := m.store.SaveServiceDEK(ctx, serviceName, newlyWrapped); err != nil {
		return nil, fmt.Errorf("secrets: save DEK for %q: %w", serviceName, err)
	}
	return raw, nil
}

func (m *Manager) masterKey() *MasterKey {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mk
}

// RotateMasterKey re-wraps every stored DEK from the active master key to
// newMasterKey (a serialized age identity), then swaps the active key. On
// any failure nothing changes: the DB rotation is one transaction and
// the in-memory key is only swapped after it commits.
func (m *Manager) RotateMasterKey(ctx context.Context, newMasterKey string) (time.Time, error) {
	newKey, err := LoadMasterKey(newMasterKey)
	if err != nil {
		return time.Time{}, fmt.Errorf("secrets: rotate master key: load new key: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := RotateStoredDEKs(ctx, m.store, m.mk, newKey); err != nil {
		return time.Time{}, err
	}
	m.mk = newKey

	rotatedAt, ok, err := m.store.GetMasterKeyRotatedAt(ctx)
	if err != nil || !ok {
		return time.Now().UTC(), nil
	}
	return rotatedAt, nil
}

// GetMasterKeyRotatedAt returns the last time RotateMasterKey succeeded,
// or ok=false if it never has.
func (m *Manager) GetMasterKeyRotatedAt(ctx context.Context) (time.Time, bool, error) {
	rotatedAt, ok, err := m.store.GetMasterKeyRotatedAt(ctx)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("secrets: get master key rotated at: %w", err)
	}
	return rotatedAt, ok, nil
}
