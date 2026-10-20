package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// oidcSigningKeyServiceName/oidcSigningKeyEnvKey are the KeyStore
// (serviceName, envKey) pair the signing key is persisted under. A fixed
// pair, not parameterized: there is only ever one active signing key per
// control plane, the same "singleton, not per-ID" reasoning
// store.GitHubAppSecretsKey's own doc comment gives.
const (
	oidcSigningKeyServiceName = "oidc/signing-key"
	oidcSigningKeyEnvKey      = "key"
)

// DefaultTTL is how long a minted token is valid for when Config.TTL is
// unset: short enough that a leaked token (a job's own logs, however
// unlikely given masking) is useless soon after the job finishes.
const DefaultTTL = 10 * time.Minute

// DefaultKeyRetireGrace is how long a rotated-out key stays published in
// the JWKS when RotateKey's retireAfter is zero: long enough that every
// token minted under it (DefaultTTL-lived) expires on its own, and that
// a verifier caching the JWKS document (AWS, GCP, and Vault all do, for
// hours) has re-fetched it at least once before the key disappears.
const DefaultKeyRetireGrace = 24 * time.Hour

// KeyStore is the narrow surface Manager needs to persist the signing
// key: internal/secrets.Manager satisfies this structurally, the same
// consumer-defined-interface shape internal/api.GitHubAppSecrets already
// establishes for that package's own private key.
type KeyStore interface {
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// Config configures a Manager.
type Config struct {
	KeyStore KeyStore
	// IssuerURL is the "iss" claim every minted token carries, and the
	// URL a verifier (AWS, GCP, Vault) is told to fetch this instance's
	// JWKS from. Must be a real, reachable HTTPS URL for federation to
	// actually work; Manager itself does not validate that, since it has
	// no way to confirm reachability from inside the process that mints
	// the tokens.
	IssuerURL string
	TTL       time.Duration
	// KeyRetireGrace is the default RotateKey uses when its own
	// retireAfter is zero. DefaultKeyRetireGrace when this is also zero.
	KeyRetireGrace time.Duration
	Now            func() time.Time
}

// retiringKey is a previously active signing key kept in the published
// JWKS so tokens it already signed keep verifying, until retireAt.
// A zero retireAt means "kept until explicitly removed."
type retiringKey struct {
	key      *Key
	retireAt time.Time
}

// Manager mints OIDC tokens and serves the matching JWKS, backed by one
// active signing key plus zero or more retiring keys, all cached in
// memory for the life of the process and persisted through KeyStore.
type Manager struct {
	cfg      Config
	mu       sync.Mutex
	active   *Key
	retiring []retiringKey
}

// NewManager builds a Manager. cfg.KeyStore and cfg.IssuerURL are
// required; NewManager returns an error rather than a Manager that fails
// on first use, so a misconfiguration is caught at startup.
func NewManager(cfg Config) (*Manager, error) {
	if cfg.KeyStore == nil {
		return nil, errors.New("oidc: KeyStore is required")
	}
	if cfg.IssuerURL == "" {
		return nil, errors.New("oidc: IssuerURL is required")
	}
	if cfg.TTL <= 0 {
		cfg.TTL = DefaultTTL
	}
	if cfg.KeyRetireGrace <= 0 {
		cfg.KeyRetireGrace = DefaultKeyRetireGrace
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Manager{cfg: cfg}, nil
}

type storedKey struct {
	KID       string    `json:"kid"`
	PEM       string    `json:"pem"`
	CreatedAt time.Time `json:"created_at"`
	RetireAt  time.Time `json:"retire_at,omitempty"`
}

// storedKeySet is the persisted shape: one active key plus any retiring
// keys still published for verification. decodeStoredKeySet also accepts
// the pre-rotation single-key shape for backward compatibility.
type storedKeySet struct {
	Active   storedKey   `json:"active"`
	Retiring []storedKey `json:"retiring,omitempty"`
}

func decodeStoredKeySet(raw string) (storedKeySet, error) {
	var set storedKeySet
	if err := json.Unmarshal([]byte(raw), &set); err == nil && set.Active.KID != "" {
		return set, nil
	}
	var legacy storedKey
	if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
		return storedKeySet{}, fmt.Errorf("oidc: decode stored signing key: %w", err)
	}
	return storedKeySet{Active: legacy}, nil
}

func toStoredKey(k *Key, retireAt time.Time, createdAt time.Time) (storedKey, error) {
	pemStr, err := k.pemEncode()
	if err != nil {
		return storedKey{}, err
	}
	return storedKey{KID: k.kid, PEM: pemStr, CreatedAt: createdAt, RetireAt: retireAt}, nil
}

// ensureLoadedLocked loads the key set from the KeyStore or generates
// and persists a fresh active key on first call. Callers must hold m.mu.
func (m *Manager) ensureLoadedLocked(ctx context.Context) error {
	if m.active != nil {
		return nil
	}

	exists, err := m.cfg.KeyStore.Exists(ctx, oidcSigningKeyServiceName, oidcSigningKeyEnvKey)
	if err != nil {
		return fmt.Errorf("oidc: check for existing signing key: %w", err)
	}
	if !exists {
		key, err := generateKey()
		if err != nil {
			return err
		}
		m.active = key
		if err := m.persistLocked(ctx); err != nil {
			m.active = nil
			return err
		}
		return nil
	}

	raw, err := m.cfg.KeyStore.Resolve(ctx, oidcSigningKeyServiceName, oidcSigningKeyEnvKey)
	if err != nil {
		return fmt.Errorf("oidc: resolve signing key: %w", err)
	}
	set, err := decodeStoredKeySet(raw)
	if err != nil {
		return err
	}
	active, err := parseKeyPEM(set.Active.KID, set.Active.PEM)
	if err != nil {
		return fmt.Errorf("oidc: parse stored signing key: %w", err)
	}
	retiring := make([]retiringKey, 0, len(set.Retiring))
	for _, rk := range set.Retiring {
		key, err := parseKeyPEM(rk.KID, rk.PEM)
		if err != nil {
			return fmt.Errorf("oidc: parse stored retiring key %q: %w", rk.KID, err)
		}
		retiring = append(retiring, retiringKey{key: key, retireAt: rk.RetireAt})
	}
	m.active = active
	m.retiring = retiring
	return nil
}

// persistLocked writes the current key set to the KeyStore. Callers must
// hold m.mu.
func (m *Manager) persistLocked(ctx context.Context) error {
	activeStored, err := toStoredKey(m.active, time.Time{}, m.cfg.Now().UTC())
	if err != nil {
		return fmt.Errorf("oidc: encode signing key for storage: %w", err)
	}
	set := storedKeySet{Active: activeStored}
	for _, rk := range m.retiring {
		rkStored, err := toStoredKey(rk.key, rk.retireAt, m.cfg.Now().UTC())
		if err != nil {
			return fmt.Errorf("oidc: encode retiring key for storage: %w", err)
		}
		set.Retiring = append(set.Retiring, rkStored)
	}
	payload, err := json.Marshal(set)
	if err != nil {
		return fmt.Errorf("oidc: encode signing key for storage: %w", err)
	}
	if err := m.cfg.KeyStore.SetValue(ctx, oidcSigningKeyServiceName, oidcSigningKeyEnvKey, string(payload)); err != nil {
		return fmt.Errorf("oidc: persist signing key: %w", err)
	}
	return nil
}

// pruneRetiringLocked drops retiring keys whose grace period has passed
// as of now. Callers must hold m.mu.
func (m *Manager) pruneRetiringLocked(now time.Time) bool {
	kept := m.retiring[:0]
	changed := false
	for _, rk := range m.retiring {
		if rk.retireAt.IsZero() || rk.retireAt.After(now) {
			kept = append(kept, rk)
			continue
		}
		changed = true
	}
	m.retiring = kept
	return changed
}

// IssueToken mints a short-lived OIDC token for req, signed with ES256
// using the current active key.
func (m *Manager) IssueToken(ctx context.Context, req TokenRequest) (string, error) {
	if req.Audience == "" {
		return "", errors.New("oidc: audience is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureLoadedLocked(ctx); err != nil {
		return "", err
	}
	now := m.cfg.Now().UTC()
	return m.active.sign(claims{
		Issuer:     m.cfg.IssuerURL,
		Subject:    req.Subject,
		Audience:   req.Audience,
		IssuedAt:   now.Unix(),
		NotBefore:  now.Unix(),
		ExpiresAt:  now.Add(m.cfg.TTL).Unix(),
		Repo:       req.Repo,
		Ref:        req.Ref,
		PipelineID: req.PipelineID,
	})
}

// JWKS returns the active key's public JWK plus every retiring key whose
// grace period has not yet passed, so a token minted moments before a
// rotation still verifies.
func (m *Manager) JWKS(ctx context.Context) (JWKS, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureLoadedLocked(ctx); err != nil {
		return JWKS{}, err
	}
	now := m.cfg.Now().UTC()
	keys := []JWK{m.active.jwk()}
	for _, rk := range m.retiring {
		if rk.retireAt.IsZero() || rk.retireAt.After(now) {
			keys = append(keys, rk.key.jwk())
		}
	}
	return JWKS{Keys: keys}, nil
}

// RotationResult is the outcome of one RotateKey call.
type RotationResult struct {
	OldKID   string
	NewKID   string
	RetireAt time.Time
	// RetiringCount is how many keys are now published in the JWKS
	// purely for verification (no longer used to sign anything new).
	RetiringCount int
}

// RotateKey generates a fresh signing key and makes it active
// immediately. The previous active key moves to the retiring set and
// stays published in the JWKS, so tokens it already signed keep
// verifying, until retireAfter elapses (DefaultKeyRetireGrace when
// retireAfter is zero). Nothing removes a retiring key before that:
// removal is the deadline passing, checked live by JWKS, not a separate
// step an operator can forget.
func (m *Manager) RotateKey(ctx context.Context, retireAfter time.Duration) (RotationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureLoadedLocked(ctx); err != nil {
		return RotationResult{}, err
	}
	if retireAfter <= 0 {
		retireAfter = m.cfg.KeyRetireGrace
	}
	now := m.cfg.Now().UTC()
	m.pruneRetiringLocked(now)

	oldActive := m.active
	newKey, err := generateKey()
	if err != nil {
		return RotationResult{}, err
	}
	retireAt := now.Add(retireAfter)

	m.active = newKey
	m.retiring = append(m.retiring, retiringKey{key: oldActive, retireAt: retireAt})
	if err := m.persistLocked(ctx); err != nil {
		m.active = oldActive
		m.retiring = m.retiring[:len(m.retiring)-1]
		return RotationResult{}, err
	}
	return RotationResult{OldKID: oldActive.kid, NewKID: newKey.kid, RetireAt: retireAt, RetiringCount: len(m.retiring)}, nil
}

// RemoveRetiringKey immediately drops a retiring key from the JWKS
// before its grace period would otherwise elapse, for an operator who
// knows a specific key is compromised and cannot wait out the normal
// propagation window. Not wired to a CLI or UI control yet: RotateKey's
// automatic, time-based removal is the path every rotation actually
// needs, and an emergency override is a deliberate follow-up, not part
// of this pass.
func (m *Manager) RemoveRetiringKey(ctx context.Context, kid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureLoadedLocked(ctx); err != nil {
		return err
	}
	kept := m.retiring[:0]
	found := false
	for _, rk := range m.retiring {
		if rk.key.kid == kid {
			found = true
			continue
		}
		kept = append(kept, rk)
	}
	if !found {
		return fmt.Errorf("oidc: no retiring key with kid %q", kid)
	}
	m.retiring = kept
	return m.persistLocked(ctx)
}
