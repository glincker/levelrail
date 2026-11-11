// Package vault resolves a live secret value from an external HashiCorp
// Vault instance at container-create time, as an alternative to
// internal/secrets' own envelope-encrypted storage. It is a sibling
// package, not a change to internal/secrets: a Vault-sourced value is
// never persisted here or anywhere else. It may be held in memory for a
// short runtime-setting TTL, the same discipline internal/secrets uses.
package vault

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	vaultapi "github.com/hashicorp/vault/api"

	"golang.org/x/sync/singleflight"

	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ErrSecretMissing marks a secret, field or version that does not exist, so callers can tell it from an outage.
var ErrSecretMissing = errors.New("vault: secret not found")

// Resolver reads secrets from a HashiCorp Vault or OpenBao KV v2 engine.
// It keeps an AppRole login token and a short read cache in memory only.
type Resolver struct {
	tunables func(context.Context) Tunables
	now      func() time.Time

	mu       sync.Mutex
	sessions map[string]session
	cache    map[string]cacheEntry
	logins   singleflight.Group
}

type session struct {
	token   string
	renewAt time.Time // zero means the token does not expire
}

type cacheEntry struct {
	value   string
	expires time.Time
}

// NewResolver returns a Resolver reading its bounds from runtime defaults and env.
func NewResolver() *Resolver { return NewResolverWith(TunablesFrom(nil)) }

// NewResolverWith returns a Resolver reading its bounds from tunables on every resolve.
func NewResolverWith(tunables func(context.Context) Tunables) *Resolver {
	return &Resolver{tunables: tunables, now: time.Now, sessions: map[string]session{}, cache: map[string]cacheEntry{}}
}

// Flush drops every cached token and value, for a connection change or rotation.
func (r *Resolver) Flush() {
	r.mu.Lock()
	r.sessions = map[string]session{}
	r.cache = map[string]cacheEntry{}
	r.mu.Unlock()
}

// fingerprint ties cache entries to the exact connection and credential, so a
// changed address, mount, role or rotated credential can never hit an old entry.
func fingerprint(cfg store.VaultSettings, credential string) string {
	h := sha256.New()
	for _, part := range []string{cfg.Address, cfg.Namespace, cfg.MountPath, cfg.AuthMethod, cfg.RoleID, credential} {
		_, _ = fmt.Fprintf(h, "%d:%s|", len(part), part)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Resolve authenticates against cfg's Vault instance using credential (a
// Vault token when cfg.AuthMethod is store.VaultAuthMethodToken, an
// AppRole secret ID when it's store.VaultAuthMethodAppRole), reads the
// KV v2 secret at path under cfg.MountPath, and returns the string value
// stored at key inside it. Never logs credential, path contents, or the
// returned value. A login failure fails the read; nothing stale is served.
func (r *Resolver) Resolve(ctx context.Context, cfg store.VaultSettings, credential, path, key string) (string, error) {
	tun := r.tunables(ctx)
	secretmask.Add(credential)
	fp := fingerprint(cfg, credential)
	cacheKey := fp + "|" + path + "|" + key
	if tun.CacheTTL > 0 {
		if v, ok := r.cached(cacheKey); ok {
			secretmask.Add(v)
			return v, nil
		}
	}

	httpClient, err := guardedClient(cfg.Address, tun.AllowPrivate, tun.Timeout)
	if err != nil {
		return "", err
	}
	client, err := vaultapi.NewClient(&vaultapi.Config{Address: cfg.Address, HttpClient: httpClient})
	if err != nil {
		return "", fmt.Errorf("vault: build client: %w", err)
	}
	if cfg.Namespace != "" {
		client.SetNamespace(cfg.Namespace)
	}

	value, err := r.read(ctx, client, cfg, credential, fp, path, key, tun)
	if err != nil {
		return "", err
	}
	secretmask.Add(value)
	if tun.CacheTTL > 0 {
		r.store(cacheKey, value, tun.CacheTTL)
	}
	return value, nil
}

func (r *Resolver) cached(k string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.cache[k]
	if !ok || !r.now().Before(e.expires) {
		delete(r.cache, k)
		return "", false
	}
	return e.value, true
}

func (r *Resolver) store(k, v string, ttl time.Duration) {
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	for old, e := range r.cache {
		if !now.Before(e.expires) {
			delete(r.cache, old)
		}
	}
	r.cache[k] = cacheEntry{value: v, expires: now.Add(ttl)}
}

// read sets the token and reads the secret; a rejected cached login is dropped and retried once.
func (r *Resolver) read(ctx context.Context, client *vaultapi.Client, cfg store.VaultSettings, credential, fp, path, key string, tun Tunables) (string, error) {
	for attempt := 0; ; attempt++ {
		token, err := r.token(ctx, client, cfg, credential, fp, tun)
		if err != nil {
			return "", err
		}
		client.SetToken(token)
		mountPath := cfg.MountPath
		if mountPath == "" {
			mountPath = "secret"
		}
		secret, err := client.KVv2(mountPath).Get(ctx, path)
		if err != nil {
			var re *vaultapi.ResponseError
			if attempt == 0 && cfg.AuthMethod == store.VaultAuthMethodAppRole && errors.As(err, &re) && re.StatusCode == http.StatusForbidden {
				r.dropSession(fp)
				continue
			}
			if errors.Is(err, vaultapi.ErrSecretNotFound) || (errors.As(err, &re) && re.StatusCode == http.StatusNotFound) {
				err = fmt.Errorf("%w: %w", ErrSecretMissing, err)
			}
			return "", fmt.Errorf("vault: read secret %q: %w", path, err)
		}
		if secret == nil {
			return "", fmt.Errorf("%w: %q", ErrSecretMissing, path)
		}
		raw, ok := secret.Data[key]
		if !ok {
			return "", fmt.Errorf("%w: %q has no field %q", ErrSecretMissing, path, key)
		}
		value, ok := raw.(string)
		if !ok {
			return "", fmt.Errorf("vault: secret %q field %q is not a string", path, key)
		}
		return value, nil
	}
}
