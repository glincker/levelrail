package secretprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/vault"
)

// OwnerKey is the internal/secrets namespace that holds every provider config and token.
const OwnerKey = "secretproviders"

const (
	cfgPrefix   = "CFG_"
	tokenPrefix = "TOKEN_" //nolint:gosec // key prefix, not a credential
	prevPrefix  = "PREV_"
)

// ErrNoProvider is returned when a reference names a provider that is not configured.
var ErrNoProvider = errors.New("secret provider is not configured on this control plane")

// Secrets is the internal/secrets.Manager surface the provider store needs.
type Secrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	ListKeys(ctx context.Context, serviceName string) ([]store.SecretKeyInfo, error)
	DeleteValue(ctx context.Context, serviceName, envKey string) error
}

// Store keeps provider configs and tokens encrypted in internal/secrets.
type Store struct {
	Secrets Secrets
}

// Names are lowercase only, so upper-casing them is collision free.
func suffix(name string) string { return strings.ToUpper(name) }

// List returns every configured provider, sorted by name.
func (s Store) List(ctx context.Context) ([]Config, error) {
	keys, err := s.Secrets.ListKeys(ctx, OwnerKey)
	if err != nil {
		return nil, fmt.Errorf("list secret providers: %w", err)
	}
	out := []Config{}
	for _, k := range keys {
		if !strings.HasPrefix(k.Key, cfgPrefix) {
			continue
		}
		cfg, ok, err := s.Get(ctx, strings.ToLower(strings.TrimPrefix(k.Key, cfgPrefix)))
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, cfg)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get returns one provider config.
func (s Store) Get(ctx context.Context, name string) (Config, bool, error) {
	if !namePattern.MatchString(name) {
		return Config{}, false, nil
	}
	ok, err := s.Secrets.Exists(ctx, OwnerKey, cfgPrefix+suffix(name))
	if err != nil || !ok {
		return Config{}, false, err
	}
	raw, err := s.Secrets.Resolve(ctx, OwnerKey, cfgPrefix+suffix(name))
	if err != nil {
		return Config{}, false, fmt.Errorf("read secret provider %q: %w", name, err)
	}
	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return Config{}, false, fmt.Errorf("decode secret provider %q: %w", name, err)
	}
	return cfg, true, nil
}

// HasToken reports whether a token is stored for name.
func (s Store) HasToken(ctx context.Context, name string) bool {
	ok, err := s.Secrets.Exists(ctx, OwnerKey, tokenPrefix+suffix(name))
	return err == nil && ok
}

// Tokens returns the current token and, inside the grace window, the previous one.
func (s Store) Tokens(ctx context.Context, cfg Config, now time.Time) (current, previous string, err error) {
	current, err = s.Secrets.Resolve(ctx, OwnerKey, tokenPrefix+suffix(cfg.Name))
	if err != nil {
		return "", "", fmt.Errorf("resolve %s provider %q token: %w", cfg.Type, cfg.Name, err)
	}
	if !cfg.GraceUntil.IsZero() && now.Before(cfg.GraceUntil) {
		previous, _ = s.Secrets.Resolve(ctx, OwnerKey, prevPrefix+suffix(cfg.Name))
	}
	return current, previous, nil
}

// Put saves cfg. An empty token keeps the stored one; a new token keeps the old one for grace.
func (s Store) Put(ctx context.Context, cfg Config, token string, grace time.Duration, now time.Time) (Config, error) {
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	if strings.HasPrefix(token, IdentityPrefix) {
		if cfg.Type != TypeInfisical {
			return Config{}, fmt.Errorf("machine identity credentials are only supported for Infisical")
		}
		if _, _, ok := ParseIdentity(token); !ok {
			return Config{}, fmt.Errorf("machine identity credential must be %s<clientId>:<clientSecret>", IdentityPrefix)
		}
	}
	existing, found, err := s.Get(ctx, cfg.Name)
	if err != nil {
		return Config{}, err
	}
	if found && existing.Type != cfg.Type {
		return Config{}, fmt.Errorf("provider %q is a %s provider; remove it first to change the type", cfg.Name, existing.Type)
	}
	cfg.GraceUntil = existing.GraceUntil
	// A stored token must never be sent to an endpoint or project it was not issued for.
	moved := found && (existing.BaseURL != cfg.BaseURL || existing.Project != cfg.Project)
	if moved {
		if token == "" {
			return Config{}, fmt.Errorf("a new token is required when base_url or project changes")
		}
		cfg.GraceUntil = time.Time{}
		if err := s.dropPrevious(ctx, cfg.Name); err != nil {
			return Config{}, err
		}
	}
	switch {
	case token == "" && !s.HasToken(ctx, cfg.Name):
		return Config{}, fmt.Errorf("token is required the first time a provider is added")
	case token != "":
		old, _, _ := s.Tokens(ctx, existing, now)
		if found && !moved && old != "" && old != token && grace > 0 {
			if err := s.Secrets.SetValue(ctx, OwnerKey, prevPrefix+suffix(cfg.Name), old); err != nil {
				return Config{}, fmt.Errorf("keep previous provider token: %w", err)
			}
			cfg.GraceUntil = now.Add(grace)
		}
		if err := s.Secrets.SetValue(ctx, OwnerKey, tokenPrefix+suffix(cfg.Name), token); err != nil {
			return Config{}, fmt.Errorf("save provider token: %w", err)
		}
	}
	cfg.UpdatedAt = now.UTC()
	raw, err := json.Marshal(cfg)
	if err != nil {
		return Config{}, fmt.Errorf("encode secret provider: %w", err)
	}
	if err := s.Secrets.SetValue(ctx, OwnerKey, cfgPrefix+suffix(cfg.Name), string(raw)); err != nil {
		return Config{}, fmt.Errorf("save secret provider: %w", err)
	}
	return cfg, nil
}

func (s Store) dropPrevious(ctx context.Context, name string) error {
	ok, err := s.Secrets.Exists(ctx, OwnerKey, prevPrefix+suffix(name))
	if err != nil || !ok {
		return err
	}
	if err := s.Secrets.DeleteValue(ctx, OwnerKey, prevPrefix+suffix(name)); err != nil {
		return fmt.Errorf("drop previous provider token: %w", err)
	}
	return nil
}

// ResolvesLive reports whether v would be read through a configured provider at container create.
// Native op:// values that name no 1Password provider stay literal, so they do not count.
func (s Store) ResolvesLive(ctx context.Context, v string) (bool, error) {
	ref, isRef, err := ParseRef(v)
	if !isRef {
		return false, nil
	}
	if !strings.HasPrefix(v, SchemeOnePass) {
		return true, nil
	}
	if err != nil {
		return false, nil
	}
	cfg, ok, gerr := s.Get(ctx, ref.Provider)
	if gerr != nil {
		return false, gerr
	}
	return ok && cfg.Type == TypeOnePassword, nil
}

// Delete removes the config and both tokens; removing a missing provider is not an error.
func (s Store) Delete(ctx context.Context, name string) error {
	if !namePattern.MatchString(name) {
		return nil
	}
	for _, p := range []string{cfgPrefix, tokenPrefix, prevPrefix} {
		ok, err := s.Secrets.Exists(ctx, OwnerKey, p+suffix(name))
		if err != nil {
			return fmt.Errorf("check secret provider %q: %w", name, err)
		}
		if !ok {
			continue
		}
		if err := s.Secrets.DeleteValue(ctx, OwnerKey, p+suffix(name)); err != nil {
			return fmt.Errorf("remove secret provider %q: %w", name, err)
		}
	}
	return nil
}

// TunablesFrom reads the read bounds from runtime settings; a nil service serves env then defaults.
func TunablesFrom(s *runtimesettings.Service) func(context.Context) Tunables {
	return func(ctx context.Context) Tunables {
		return Tunables{
			Timeout:          time.Duration(s.Int(ctx, runtimesettings.KeySecretProviderTimeout)) * time.Second,
			CacheTTL:         time.Duration(s.Int(ctx, runtimesettings.KeySecretProviderCacheTTL)) * time.Second,
			MaxResponseBytes: int64(s.Int(ctx, runtimesettings.KeySecretProviderMaxResponse)) * 1024,
			MaxRPS:           s.Int(ctx, runtimesettings.KeySecretProviderMaxRPS),
		}
	}
}

// SecretResolver is the reconciler's secret read surface.
type SecretResolver interface {
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// EnvResolver wraps the app secret resolver so a stored doppler://, infisical:// or op:// provider value resolves live.
type EnvResolver struct {
	Inner     SecretResolver
	Providers Store
	Resolver  *Resolver
	Tunables  func(context.Context) Tunables
}

// Exists delegates to the wrapped resolver.
func (e EnvResolver) Exists(ctx context.Context, serviceName, envKey string) (bool, error) {
	return e.Inner.Exists(ctx, serviceName, envKey)
}

// Resolve returns the stored value, or the live provider value when it is a reference. Fails closed.
func (e EnvResolver) Resolve(ctx context.Context, serviceName, envKey string) (string, error) {
	v, err := e.Inner.Resolve(ctx, serviceName, envKey)
	if err != nil {
		return "", err
	}
	ref, isRef, err := ParseRef(v)
	if !isRef {
		return v, nil
	}
	if ref.Type == TypeOnePassword || (err != nil && strings.HasPrefix(v, SchemeOnePass)) {
		// Native op:// values for an in-container CLI stay literal unless they name a configured provider.
		if err != nil {
			return v, nil
		}
		cfg, ok, gerr := e.Providers.Get(ctx, ref.Provider)
		if gerr != nil {
			return "", gerr
		}
		if !ok || cfg.Type != TypeOnePassword {
			return v, nil
		}
	}
	if err != nil {
		return "", fmt.Errorf("secret %q holds a bad provider reference: %w", envKey, err)
	}
	value, err := e.ResolveRef(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("secret %q (%s): %w", envKey, ref, err)
	}
	return value, nil
}

// ResolveRef reads one reference, retrying with the previous token inside its grace window.
func (e EnvResolver) ResolveRef(ctx context.Context, ref Ref) (string, error) {
	cfg, ok, err := e.Providers.Get(ctx, ref.Provider)
	if err != nil {
		return "", err
	}
	if !ok || cfg.Type != ref.Type {
		return "", fmt.Errorf("%s provider %q: %w", ref.Type, ref.Provider, ErrNoProvider)
	}
	now := time.Now()
	current, previous, err := e.Providers.Tokens(ctx, cfg, now)
	if err != nil {
		return "", err
	}
	t := e.Tunables(ctx)
	return vault.ResolveWithGrace(now, cfg.GraceUntil, current, previous, func(token string) (string, error) {
		return e.Resolver.Fetch(ctx, cfg, token, ref, t)
	})
}
