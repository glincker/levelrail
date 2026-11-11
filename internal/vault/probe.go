package vault

import (
	"context"
	"fmt"

	vaultapi "github.com/hashicorp/vault/api"

	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Probe checks that the address is reachable and the credential authenticates,
// by looking up the caller's own token. It reads no secret and returns no data.
func (r *Resolver) Probe(ctx context.Context, cfg store.VaultSettings, credential string) error {
	tun := r.tunables(ctx)
	secretmask.Add(credential)
	httpClient, err := guardedClient(cfg.Address, tun.AllowPrivate, tun.Timeout)
	if err != nil {
		return err
	}
	client, err := vaultapi.NewClient(&vaultapi.Config{Address: cfg.Address, HttpClient: httpClient})
	if err != nil {
		return fmt.Errorf("vault: build client: %w", err)
	}
	if cfg.Namespace != "" {
		client.SetNamespace(cfg.Namespace)
	}
	token, err := r.token(ctx, client, cfg, credential, fingerprint(cfg, credential), tun)
	if err != nil {
		return err
	}
	client.SetToken(token)
	if _, err := client.Auth().Token().LookupSelfWithContext(ctx); err != nil {
		return fmt.Errorf("vault: token check: %w", err)
	}
	return nil
}
