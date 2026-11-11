package secretconn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretprovider"
	"github.com/GLINCKER/levelrail/internal/secretrotation"
	"github.com/GLINCKER/levelrail/internal/secretsource"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/vault"
)

// Connection types.
const (
	TypeVault       = "vault"
	TypeDoppler     = secretprovider.TypeDoppler
	TypeInfisical   = secretprovider.TypeInfisical
	TypeOnePassword = secretprovider.TypeOnePassword
)

// LegacyVaultName is the implicit connection backed by the single Vault settings row.
const LegacyVaultName = "vault"

// SecretsOwner is the internal/secrets namespace holding stored connection credentials.
const SecretsOwner = "secretconnections" //nolint:gosec // namespace name, not a credential

// Errors the API maps to status codes.
var (
	ErrInvalid  = errors.New("secretconn: invalid connection")
	ErrConflict = errors.New("secretconn: connection name already in use")
	ErrNotFound = errors.New("secretconn: connection not found")
	ErrLegacy   = errors.New("secretconn: legacy connection is managed through its own settings")
)

// Config holds the non-secret settings; which fields apply depends on Type.
type Config struct {
	Address     string `json:"address,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	Mount       string `json:"mount,omitempty"`
	AuthMethod  string `json:"auth_method,omitempty"`
	RoleID      string `json:"role_id,omitempty"`
	Project     string `json:"project,omitempty"`
	Environment string `json:"environment,omitempty"`
	Path        string `json:"path,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`
}

// Connection is one named source; it never carries a credential value.
type Connection struct {
	Name          string
	Type          string
	Legacy        bool
	Config        Config
	HasCredential bool
	CredentialRef string
	CreatedBy     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Input creates or updates a connection. Credential is write-only; empty keeps the stored one on update.
type Input struct {
	Type          string
	Config        Config
	Credential    string
	CredentialRef string
}

// TestResult is the outcome of a health probe. Reason is a class, never provider output.
type TestResult struct {
	OK     bool
	Class  string
	Reason string
}

// Rows is the connection table.
type Rows interface {
	ListSecretConnections(ctx context.Context) ([]store.SecretConnection, error)
	GetSecretConnection(ctx context.Context, name string) (store.SecretConnection, error)
	CreateSecretConnection(ctx context.Context, c store.SecretConnection) error
	UpdateSecretConnection(ctx context.Context, c store.SecretConnection) error
	DeleteSecretConnection(ctx context.Context, name string) error
}

// VaultSettings reads the legacy single Vault row.
type VaultSettings interface {
	GetVaultSettings(ctx context.Context) (store.VaultSettings, error)
}

// Service owns the connection set and keeps a secretsource.Resolver in step with it.
type Service struct {
	Rows     Rows
	Secrets  secretprovider.Secrets
	Vault    VaultSettings
	Sources  *secretsource.Resolver
	VaultRes *vault.Resolver
	ProvRes  *secretprovider.Resolver
	ProvTun  func(context.Context) secretprovider.Tunables
	Now      func() time.Time
	// Build overrides adapter construction, for tests only.
	Build func(Connection) secretsource.Provider

	mu         sync.Mutex
	registered map[string]bool
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) providers() secretprovider.Store { return secretprovider.Store{Secrets: s.Secrets} }

func tokenKey(name string) string { return "TOKEN_" + strings.ToUpper(name) }

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// List returns legacy connections first, then stored ones, each group sorted by name.
func (s *Service) List(ctx context.Context) ([]Connection, error) {
	out, err := s.legacy(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.Rows.ListSecretConnections(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, c := range out {
		seen[c.Name] = true
	}
	for _, r := range rows {
		if seen[r.Name] {
			continue // a legacy name wins so existing references keep their meaning
		}
		out = append(out, s.fromRow(ctx, r))
	}
	return out, nil
}

// Get returns one connection by name.
func (s *Service) Get(ctx context.Context, name string) (Connection, error) {
	all, err := s.List(ctx)
	if err != nil {
		return Connection{}, err
	}
	for _, c := range all {
		if c.Name == name {
			return c, nil
		}
	}
	return Connection{}, ErrNotFound
}

func (s *Service) legacy(ctx context.Context) ([]Connection, error) {
	var out []Connection
	if s.Vault != nil {
		v, err := s.Vault.GetVaultSettings(ctx)
		if err != nil {
			return nil, err
		}
		if v.Enabled && v.Address != "" {
			has, _ := s.Secrets.Exists(ctx, store.VaultSecretsKey(), store.VaultCredentialEnvKey)
			out = append(out, Connection{
				Name: LegacyVaultName, Type: TypeVault, Legacy: true, HasCredential: has,
				Config: Config{Address: v.Address, Namespace: v.Namespace, Mount: v.MountPath, AuthMethod: v.AuthMethod, RoleID: v.RoleID},
			})
		}
	}
	cfgs, err := s.providers().List(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range cfgs {
		out = append(out, Connection{
			Name: c.Name, Type: c.Type, Legacy: true, HasCredential: s.providers().HasToken(ctx, c.Name),
			Config: Config{Project: c.Project, Environment: c.Environment, Path: c.Path, BaseURL: c.BaseURL}, UpdatedAt: c.UpdatedAt,
		})
	}
	return out, nil
}

func (s *Service) fromRow(ctx context.Context, r store.SecretConnection) Connection {
	var cfg Config
	_ = json.Unmarshal([]byte(r.Config), &cfg)
	has := r.CredentialRef != ""
	if !has {
		has, _ = s.Secrets.Exists(ctx, SecretsOwner, tokenKey(r.Name))
	}
	return Connection{Name: r.Name, Type: r.Type, Config: cfg, HasCredential: has, CredentialRef: r.CredentialRef,
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func validateConfig(typ string, c Config) (Config, error) {
	switch typ {
	case TypeVault:
		u, err := url.Parse(c.Address)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
			return c, invalid("address must be an http or https URL with a host and no credentials")
		}
		if c.AuthMethod == "" {
			c.AuthMethod = store.VaultAuthMethodToken
		}
		if c.AuthMethod != store.VaultAuthMethodToken && c.AuthMethod != store.VaultAuthMethodAppRole {
			return c, invalid("auth_method must be token or approle")
		}
		if c.AuthMethod == store.VaultAuthMethodAppRole && c.RoleID == "" {
			return c, invalid("role_id is required for approle")
		}
		if c.Mount == "" {
			c.Mount = "secret"
		}
		return Config{Address: c.Address, Namespace: c.Namespace, Mount: c.Mount, AuthMethod: c.AuthMethod, RoleID: c.RoleID}, nil
	case TypeDoppler, TypeInfisical, TypeOnePassword:
		// The provider rules need a legal provider name, which a connection name may not be.
		pc := secretprovider.Config{Name: "conn", Type: typ, Project: c.Project, Environment: c.Environment, Path: c.Path, BaseURL: c.BaseURL}
		if err := pc.Validate(); err != nil {
			return c, invalid("%s", err.Error())
		}
		return Config{Project: c.Project, Environment: c.Environment, Path: c.Path, BaseURL: c.BaseURL}, nil
	}
	return c, invalid("type must be vault, doppler, infisical or onepassword")
}

func validateCredential(typ, cred string) error {
	if strings.HasPrefix(cred, secretprovider.IdentityPrefix) {
		if typ != TypeInfisical {
			return invalid("machine identity credentials are only supported for Infisical")
		}
		if _, _, ok := secretprovider.ParseIdentity(cred); !ok {
			return invalid("machine identity credential must be %s<clientId>:<clientSecret>", secretprovider.IdentityPrefix)
		}
	}
	return nil
}

func (s *Service) checkRef(ctx context.Context, ref string) error {
	svc, key, ok := strings.Cut(ref, "/")
	if !ok || svc == "" || key == "" {
		return invalid("credential_ref must be <secret namespace>/<key>")
	}
	if svc == SecretsOwner {
		return invalid("credential_ref cannot point at another connection's stored credential")
	}
	exists, err := s.Secrets.Exists(ctx, svc, key)
	if err != nil {
		return err
	}
	if !exists {
		return invalid("credential_ref does not name an existing secret")
	}
	return nil
}

// Create stores a new named connection and registers it.
func (s *Service) Create(ctx context.Context, actor, name string, in Input) (Connection, error) {
	if err := secretsource.ValidConnName(name, secretsource.CurrentLimits().MaxConn); err != nil {
		return Connection{}, invalid("name must be lowercase [a-z0-9_-], starting alphanumeric, within the length limit")
	}
	if _, err := s.Get(ctx, name); err == nil {
		return Connection{}, ErrConflict
	} else if !errors.Is(err, ErrNotFound) {
		return Connection{}, err
	}
	if name == LegacyVaultName {
		return Connection{}, ErrConflict
	}
	cfg, err := validateConfig(in.Type, in.Config)
	if err != nil {
		return Connection{}, err
	}
	if (in.Credential == "") == (in.CredentialRef == "") {
		return Connection{}, invalid("give exactly one of credential and credential_ref")
	}
	if err := validateCredential(in.Type, in.Credential); err != nil {
		return Connection{}, err
	}
	if in.CredentialRef != "" {
		if err := s.checkRef(ctx, in.CredentialRef); err != nil {
			return Connection{}, err
		}
	}
	raw, _ := json.Marshal(cfg)
	row := store.SecretConnection{Name: name, Type: in.Type, Config: string(raw), CredentialRef: in.CredentialRef, CreatedBy: actor}
	if in.Credential != "" {
		if err := s.Secrets.SetValue(ctx, SecretsOwner, tokenKey(name), in.Credential); err != nil {
			return Connection{}, fmt.Errorf("save connection credential: %w", err)
		}
	}
	if err := s.Rows.CreateSecretConnection(ctx, row); err != nil {
		_ = s.Secrets.DeleteValue(ctx, SecretsOwner, tokenKey(name))
		if errors.Is(err, store.ErrSecretConnectionExists) {
			return Connection{}, ErrConflict
		}
		return Connection{}, err
	}
	if err := s.Sync(ctx); err != nil {
		return Connection{}, err
	}
	return s.Get(ctx, name)
}

// Update replaces config and optionally the credential; the type never changes.
func (s *Service) Update(ctx context.Context, name string, in Input) (Connection, error) {
	cur, err := s.Get(ctx, name)
	if err != nil {
		return Connection{}, err
	}
	if cur.Legacy {
		return Connection{}, ErrLegacy
	}
	if in.Type != "" && in.Type != cur.Type {
		return Connection{}, invalid("type cannot change; delete the connection and create it again")
	}
	cfg, err := validateConfig(cur.Type, in.Config)
	if err != nil {
		return Connection{}, err
	}
	if in.Credential != "" && in.CredentialRef != "" {
		return Connection{}, invalid("give at most one of credential and credential_ref")
	}
	if err := validateCredential(cur.Type, in.Credential); err != nil {
		return Connection{}, err
	}
	// A stored credential must never be sent to an endpoint or scope it was not issued for.
	if in.Credential == "" && in.CredentialRef == "" && cfg != cur.Config {
		return Connection{}, invalid("a new credential is required when the connection settings change")
	}
	ref := cur.CredentialRef
	if in.CredentialRef != "" {
		if err := s.checkRef(ctx, in.CredentialRef); err != nil {
			return Connection{}, err
		}
		ref = in.CredentialRef
	}
	if in.Credential != "" {
		if err := s.Secrets.SetValue(ctx, SecretsOwner, tokenKey(name), in.Credential); err != nil {
			return Connection{}, fmt.Errorf("save connection credential: %w", err)
		}
		ref = ""
	}
	raw, _ := json.Marshal(cfg)
	if err := s.Rows.UpdateSecretConnection(ctx, store.SecretConnection{Name: name, Config: string(raw), CredentialRef: ref}); err != nil {
		if errors.Is(err, store.ErrSecretConnectionNotFound) {
			return Connection{}, ErrNotFound
		}
		return Connection{}, err
	}
	if in.CredentialRef != "" {
		_ = s.Secrets.DeleteValue(ctx, SecretsOwner, tokenKey(name))
	}
	if err := s.Sync(ctx); err != nil {
		return Connection{}, err
	}
	return s.Get(ctx, name)
}

// Delete removes a stored connection, its stored credential and its registration.
func (s *Service) Delete(ctx context.Context, name string) error {
	cur, err := s.Get(ctx, name)
	if err != nil {
		return err
	}
	if cur.Legacy {
		return ErrLegacy
	}
	if err := s.Rows.DeleteSecretConnection(ctx, name); err != nil {
		if errors.Is(err, store.ErrSecretConnectionNotFound) {
			return ErrNotFound
		}
		return err
	}
	if ok, _ := s.Secrets.Exists(ctx, SecretsOwner, tokenKey(name)); ok {
		if err := s.Secrets.DeleteValue(ctx, SecretsOwner, tokenKey(name)); err != nil {
			return fmt.Errorf("remove connection credential: %w", err)
		}
	}
	return s.Sync(ctx)
}

// Test runs the provider's read-only probe and reports a class, never provider output.
func (s *Service) Test(ctx context.Context, name string) (TestResult, error) {
	c, err := s.Get(ctx, name)
	if err != nil {
		return TestResult{}, err
	}
	p := s.provider(c)
	pr, ok := p.(Prober)
	if !ok {
		return TestResult{Class: "unsupported", Reason: "this connection type has no health probe"}, nil
	}
	if err := pr.Probe(ctx); err != nil {
		return failure(err), nil
	}
	return TestResult{OK: true, Class: "ok"}, nil
}

func failure(err error) TestResult {
	switch {
	case errors.Is(err, secretsource.ErrDenied):
		return TestResult{Class: "denied", Reason: "the source refused the credential or the address"}
	case errors.Is(err, secretsource.ErrNotFound):
		return TestResult{Class: "not_found", Reason: "the source answered not found"}
	default:
		return TestResult{Class: "unavailable", Reason: "the source did not answer or answered with an error"}
	}
}

// Sync makes the secretsource.Resolver match the current connections. Adapters read their
// config live, so a re-registration is only needed when a name appears or disappears.
func (s *Service) Sync(ctx context.Context) error {
	all, err := s.List(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.registered == nil {
		s.registered = map[string]bool{}
	}
	want := map[string]bool{}
	for _, c := range all {
		want[c.Name] = true
		s.Sources.Unregister(c.Name)
		if err := s.Sources.Register(c.Name, s.provider(c)); err != nil {
			// A name the current limits refuse stays unregistered; its refs fail closed.
			delete(s.registered, c.Name)
			continue
		}
		s.registered[c.Name] = true
	}
	for name := range s.registered {
		if !want[name] {
			s.Sources.Unregister(name)
			delete(s.registered, name)
		}
	}
	return nil
}

func (s *Service) provider(c Connection) secretsource.Provider {
	if s.Build != nil {
		return s.Build(c)
	}
	now := s.now
	if c.Type == TypeVault {
		return &vaultProvider{name: c.Name, load: s.vaultLoader(c), res: s.VaultRes, now: now}
	}
	return &providerAdapter{name: c.Name, typ: c.Type, load: s.providerLoader(c), res: s.ProvRes, tun: s.ProvTun, now: now}
}

func (s *Service) vaultLoader(c Connection) vaultLoad {
	if c.Legacy {
		return func(ctx context.Context) (store.VaultSettings, string, string, time.Time, error) {
			v, err := s.Vault.GetVaultSettings(ctx)
			if err != nil || !v.Enabled {
				return store.VaultSettings{}, "", "", time.Time{}, errors.New("vault is not enabled")
			}
			cur, err := s.Secrets.Resolve(ctx, store.VaultSecretsKey(), store.VaultCredentialEnvKey)
			if err != nil {
				return store.VaultSettings{}, "", "", time.Time{}, err
			}
			var prev string
			if !v.PreviousExpiresAt.IsZero() {
				prev, _ = s.Secrets.Resolve(ctx, store.VaultSecretsKey(), store.VaultCredentialEnvKey+secretrotation.PreviousSuffix)
			}
			return v, cur, prev, v.PreviousExpiresAt, nil
		}
	}
	return func(ctx context.Context) (store.VaultSettings, string, string, time.Time, error) {
		row, err := s.Rows.GetSecretConnection(ctx, c.Name)
		if err != nil {
			return store.VaultSettings{}, "", "", time.Time{}, err
		}
		live := s.fromRow(ctx, row)
		cred, err := s.rowCredential(ctx, row)
		if err != nil {
			return store.VaultSettings{}, "", "", time.Time{}, err
		}
		cfg := store.VaultSettings{Enabled: true, Address: live.Config.Address, AuthMethod: live.Config.AuthMethod,
			Namespace: live.Config.Namespace, RoleID: live.Config.RoleID, MountPath: live.Config.Mount}
		return cfg, cred, "", time.Time{}, nil
	}
}

func (s *Service) providerLoader(c Connection) providerLoad {
	if c.Legacy {
		return func(ctx context.Context) (secretprovider.Config, string, string, error) {
			cfg, ok, err := s.providers().Get(ctx, c.Name)
			if err != nil || !ok {
				return secretprovider.Config{}, "", "", errors.New("provider is not configured")
			}
			cur, prev, err := s.providers().Tokens(ctx, cfg, s.now())
			return cfg, cur, prev, err
		}
	}
	return func(ctx context.Context) (secretprovider.Config, string, string, error) {
		row, err := s.Rows.GetSecretConnection(ctx, c.Name)
		if err != nil {
			return secretprovider.Config{}, "", "", err
		}
		live := s.fromRow(ctx, row)
		cred, err := s.rowCredential(ctx, row)
		if err != nil {
			return secretprovider.Config{}, "", "", err
		}
		cfg := secretprovider.Config{Name: c.Name, Type: c.Type, Project: live.Config.Project, Environment: live.Config.Environment,
			Path: live.Config.Path, BaseURL: live.Config.BaseURL, UpdatedAt: row.UpdatedAt}
		return cfg, cred, "", nil
	}
}

func (s *Service) rowCredential(ctx context.Context, r store.SecretConnection) (string, error) {
	if r.CredentialRef != "" {
		svc, key, _ := strings.Cut(r.CredentialRef, "/")
		return s.Secrets.Resolve(ctx, svc, key)
	}
	return s.Secrets.Resolve(ctx, SecretsOwner, tokenKey(r.Name))
}
