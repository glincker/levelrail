// Package vault resolves a live secret value from an external HashiCorp
// Vault instance at container-create time, as an alternative to
// internal/secrets' own envelope-encrypted storage. It is a sibling
// package, not a change to internal/secrets: a Vault-sourced value is
// never persisted here or anywhere else, only read fresh on every
// resolve and handed straight to the caller, the same discipline
// internal/secrets.Manager.Resolve already holds.
package vault

import (
	"context"
	"fmt"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/api/auth/approle"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Resolver reads secrets from a HashiCorp Vault KV v2 engine. The zero
// value is ready to use: it holds no state between calls, so a fresh
// vaultapi.Client is authenticated on every Resolve.
type Resolver struct{}

// NewResolver returns a ready-to-use Resolver.
func NewResolver() *Resolver {
	return &Resolver{}
}

// Resolve authenticates against cfg's Vault instance using credential (a
// Vault token when cfg.AuthMethod is store.VaultAuthMethodToken, an
// AppRole secret ID when it's store.VaultAuthMethodAppRole), reads the
// KV v2 secret at path under cfg.MountPath, and returns the string value
// stored at key inside it. Never logs credential, path contents, or the
// returned value.
func (r *Resolver) Resolve(ctx context.Context, cfg store.VaultSettings, credential, path, key string) (string, error) {
	client, err := vaultapi.NewClient(&vaultapi.Config{Address: cfg.Address})
	if err != nil {
		return "", fmt.Errorf("vault: build client: %w", err)
	}
	if cfg.Namespace != "" {
		client.SetNamespace(cfg.Namespace)
	}

	if err := authenticate(ctx, client, cfg, credential); err != nil {
		return "", err
	}

	mountPath := cfg.MountPath
	if mountPath == "" {
		mountPath = "secret"
	}

	secret, err := client.KVv2(mountPath).Get(ctx, path)
	if err != nil {
		return "", fmt.Errorf("vault: read secret %q: %w", path, err)
	}
	if secret == nil {
		return "", fmt.Errorf("vault: secret %q not found", path)
	}

	raw, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("vault: secret %q has no field %q", path, key)
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("vault: secret %q field %q is not a string", path, key)
	}
	return value, nil
}

// authenticate sets client's token, either directly (token auth) or by
// logging in via AppRole first (approle auth).
func authenticate(ctx context.Context, client *vaultapi.Client, cfg store.VaultSettings, credential string) error {
	switch cfg.AuthMethod {
	case store.VaultAuthMethodAppRole:
		auth, err := approle.NewAppRoleAuth(cfg.RoleID, &approle.SecretID{FromString: credential})
		if err != nil {
			return fmt.Errorf("vault: build approle auth: %w", err)
		}
		loginSecret, err := client.Auth().Login(ctx, auth)
		if err != nil {
			return fmt.Errorf("vault: approle login: %w", err)
		}
		if loginSecret == nil || loginSecret.Auth == nil || loginSecret.Auth.ClientToken == "" {
			return fmt.Errorf("vault: approle login returned no client token")
		}
		client.SetToken(loginSecret.Auth.ClientToken)
	default:
		client.SetToken(credential)
	}
	return nil
}
