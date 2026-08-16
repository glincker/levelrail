package secrets

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Store is the narrow surface Manager needs from internal/store, so
// tests can fake it without a real database. *store.DB satisfies this
// structurally, the same "narrow interface at the boundary" shape every
// other package here uses (application.ServiceStore, deploy.ServiceStore,
// and so on). This does mean internal/secrets depends on internal/store,
// unlike the rest of this package (the primitives in masterkey.go,
// dek.go, value.go know nothing about storage): store.ErrServiceDEKNotFound
// and store.ErrSecretValueNotFound are real sentinels this file checks
// with errors.Is, and every other consumer package (internal/deploy,
// internal/reconcile/application, internal/api) already imports
// internal/store directly for exactly this reason. internal/store does
// not import internal/secrets, so this stays one-directional, no cycle.
type Store interface {
	GetServiceDEK(ctx context.Context, serviceName string) ([]byte, error)
	SaveServiceDEK(ctx context.Context, serviceName string, wrappedDEK []byte) error
	GetSecretValue(ctx context.Context, serviceName, envKey string) ([]byte, error)
	SaveSecretValue(ctx context.Context, serviceName, envKey string, ciphertext []byte) error
	HasSecretValue(ctx context.Context, serviceName, envKey string) (bool, error)
	DeleteServiceSecrets(ctx context.Context, serviceName string) error
}

// ErrValueNotFound means no secret value has been set for a given
// (service, env key) pair, returned by Resolve regardless of whether
// the underlying cause was a missing DEK (no value ever set for this
// service) or a missing ciphertext (DEK exists, this particular key
// doesn't): callers only need to know "nothing to resolve", not which.
var ErrValueNotFound = errors.New("secrets: value not found")

// Manager combines a MasterKey with a Store to provide per-app envelope
// encryption end to end: generate-or-reuse a service's DEK, encrypt a
// value under it, persist only ciphertext and a wrapped key, and
// reverse all of that to get a plaintext value back. Callers never see
// a raw DEK or handle wrapping/unwrapping themselves.
type Manager struct {
	store Store
	mk    *MasterKey
}

// NewManager builds a Manager. mk is the control plane's master key,
// held only in memory (every DEK is wrapped by a master key held only
// by the control plane), sourced by the caller the same way
// MasterKey.String/LoadMasterKey already document (file, env, or a
// future KMS interface), never by this package.
func NewManager(store Store, mk *MasterKey) *Manager {
	return &Manager{store: store, mk: mk}
}

// SetValue encrypts plaintext under serviceName's DEK, generating one on
// first use, and persists only the ciphertext (and, on first use, the
// wrapped DEK). The plaintext itself is never persisted anywhere by this
// call.
func (m *Manager) SetValue(ctx context.Context, serviceName, envKey, plaintext string) error {
	dek, err := m.dekFor(ctx, serviceName)
	if err != nil {
		return err
	}

	ciphertext, err := EncryptValue(dek, plaintext)
	if err != nil {
		return fmt.Errorf("secrets: encrypt value for %q/%q: %w", serviceName, envKey, err)
	}

	if err := m.store.SaveSecretValue(ctx, serviceName, envKey, ciphertext); err != nil {
		return fmt.Errorf("secrets: save value for %q/%q: %w", serviceName, envKey, err)
	}
	return nil
}

// Resolve decrypts and returns the plaintext value for (serviceName,
// envKey), or ErrValueNotFound if none was ever set. Callers (the
// application controller, immediately before docker.Runtime.Create) must
// never persist what this returns.
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

	dek, err := m.mk.UnwrapDEK(WrappedDEK(wrapped))
	if err != nil {
		return "", fmt.Errorf("secrets: unwrap DEK for %q: %w", serviceName, err)
	}

	plaintext, err := DecryptValue(dek, ciphertext)
	if err != nil {
		return "", fmt.Errorf("secrets: decrypt value for %q/%q: %w", serviceName, envKey, err)
	}
	return plaintext, nil
}

// Exists reports whether a value has been set for (serviceName, envKey),
// without decrypting it. internal/deploy uses this to fail a deploy
// loudly when a { secret: true, required: true } env var has no value
// yet, rather than deferring the check to container-create time.
func (m *Manager) Exists(ctx context.Context, serviceName, envKey string) (bool, error) {
	ok, err := m.store.HasSecretValue(ctx, serviceName, envKey)
	if err != nil {
		return false, fmt.Errorf("secrets: check value for %q/%q: %w", serviceName, envKey, err)
	}
	return ok, nil
}

// DeleteAll permanently removes every value and the wrapped DEK for
// serviceName, so nothing set under it (via any past SetValue call) can
// ever be decrypted again, even if the underlying ciphertext somehow
// survives elsewhere. For a sentinel key like
// store.GitHubAppSecretsKey() this deletes the whole logical secret
// bundle at once, not one field at a time.
func (m *Manager) DeleteAll(ctx context.Context, serviceName string) error {
	if err := m.store.DeleteServiceSecrets(ctx, serviceName); err != nil {
		return fmt.Errorf("secrets: delete all values for %q: %w", serviceName, err)
	}
	return nil
}

// dekFor returns serviceName's raw DEK, generating and persisting a new
// wrapped one on first use. A service's DEK is created at most once:
// every value it ever encrypts is encrypted under the same key, so the
// DEK is wrapped once and reused to encrypt many values fast, and this
// never overwrites an existing wrapped DEK.
func (m *Manager) dekFor(ctx context.Context, serviceName string) ([]byte, error) {
	wrapped, err := m.store.GetServiceDEK(ctx, serviceName)
	if err == nil {
		return m.mk.UnwrapDEK(WrappedDEK(wrapped))
	}
	if !errors.Is(err, store.ErrServiceDEKNotFound) {
		return nil, fmt.Errorf("secrets: get DEK for %q: %w", serviceName, err)
	}

	raw, newlyWrapped, err := m.mk.GenerateDEK()
	if err != nil {
		return nil, fmt.Errorf("secrets: generate DEK for %q: %w", serviceName, err)
	}
	if err := m.store.SaveServiceDEK(ctx, serviceName, newlyWrapped); err != nil {
		return nil, fmt.Errorf("secrets: save DEK for %q: %w", serviceName, err)
	}
	return raw, nil
}
