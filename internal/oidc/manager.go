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
// pair, not parameterized: there is only ever one signing key per
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
	Now       func() time.Time
}

// Manager mints OIDC tokens and serves the matching JWKS, backed by one
// lazily generated-or-loaded signing key cached in memory for the life of
// the process.
type Manager struct {
	cfg Config
	mu  sync.Mutex
	key *Key
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
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Manager{cfg: cfg}, nil
}

type storedKey struct {
	KID       string    `json:"kid"`
	PEM       string    `json:"pem"`
	CreatedAt time.Time `json:"created_at"`
}

// ensureKey returns the cached signing key, loading it from the
// KeyStore or generating and persisting a new one on first call.
func (m *Manager) ensureKey(ctx context.Context) (*Key, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.key != nil {
		return m.key, nil
	}

	exists, err := m.cfg.KeyStore.Exists(ctx, oidcSigningKeyServiceName, oidcSigningKeyEnvKey)
	if err != nil {
		return nil, fmt.Errorf("oidc: check for existing signing key: %w", err)
	}
	if exists {
		raw, err := m.cfg.KeyStore.Resolve(ctx, oidcSigningKeyServiceName, oidcSigningKeyEnvKey)
		if err != nil {
			return nil, fmt.Errorf("oidc: resolve signing key: %w", err)
		}
		var sk storedKey
		if err := json.Unmarshal([]byte(raw), &sk); err != nil {
			return nil, fmt.Errorf("oidc: decode stored signing key: %w", err)
		}
		key, err := parseKeyPEM(sk.KID, sk.PEM)
		if err != nil {
			return nil, fmt.Errorf("oidc: parse stored signing key: %w", err)
		}
		m.key = key
		return m.key, nil
	}

	key, err := generateKey()
	if err != nil {
		return nil, err
	}
	pemStr, err := key.pemEncode()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(storedKey{KID: key.kid, PEM: pemStr, CreatedAt: m.cfg.Now().UTC()})
	if err != nil {
		return nil, fmt.Errorf("oidc: encode signing key for storage: %w", err)
	}
	if err := m.cfg.KeyStore.SetValue(ctx, oidcSigningKeyServiceName, oidcSigningKeyEnvKey, string(payload)); err != nil {
		return nil, fmt.Errorf("oidc: persist signing key: %w", err)
	}
	m.key = key
	return m.key, nil
}

// IssueToken mints a short-lived OIDC token for req, signed with ES256.
func (m *Manager) IssueToken(ctx context.Context, req TokenRequest) (string, error) {
	if req.Audience == "" {
		return "", errors.New("oidc: audience is required")
	}
	key, err := m.ensureKey(ctx)
	if err != nil {
		return "", err
	}
	now := m.cfg.Now().UTC()
	return key.sign(claims{
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

// JWKS returns the current signing key's public JWKS document.
func (m *Manager) JWKS(ctx context.Context) (JWKS, error) {
	key, err := m.ensureKey(ctx)
	if err != nil {
		return JWKS{}, err
	}
	return JWKS{Keys: []JWK{key.jwk()}}, nil
}
