package vault

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	vaultapi "github.com/hashicorp/vault/api"

	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ErrPathDenied marks a path the credential's policy does not allow reading.
var ErrPathDenied = errors.New("vault: policy does not allow reading the path")

// Probe checks that the address is reachable and the credential authenticates,
// by looking up the caller's own token. It reads no secret and returns no data.
func (r *Resolver) Probe(ctx context.Context, cfg store.VaultSettings, credential string) error {
	client, err := r.authedClient(ctx, cfg, credential)
	if err != nil {
		return err
	}
	if _, err := client.Auth().Token().LookupSelfWithContext(ctx); err != nil {
		return fmt.Errorf("vault: token check: %w", err)
	}
	return nil
}

// ProbePath is Probe plus a read-only check of one KV v2 path: the policy's
// capabilities on it, then its metadata. It never reads secret data.
func (r *Resolver) ProbePath(ctx context.Context, cfg store.VaultSettings, credential, path string) error {
	if err := r.Probe(ctx, cfg, credential); err != nil {
		return err
	}
	client, err := r.authedClient(ctx, cfg, credential)
	if err != nil {
		return err
	}
	mount := strings.Trim(cfg.MountPath, "/")
	if mount == "" {
		mount = "secret"
	}
	path = strings.Trim(path, "/")
	caps, err := client.Sys().CapabilitiesSelfWithContext(ctx, mount+"/data/"+path)
	if err != nil {
		return fmt.Errorf("vault: capabilities check: %w", err)
	}
	if !slices.Contains(caps, "read") && !slices.Contains(caps, "root") {
		return ErrPathDenied
	}
	// Metadata may be outside the policy, so only a clean not-found is a verdict.
	meta, err := client.Logical().ReadWithContext(ctx, mount+"/metadata/"+path)
	var re *vaultapi.ResponseError
	if (err == nil && meta == nil) || (errors.As(err, &re) && re.StatusCode == http.StatusNotFound) {
		return fmt.Errorf("%w: %q", ErrSecretMissing, path)
	}
	return nil
}

func (r *Resolver) authedClient(ctx context.Context, cfg store.VaultSettings, credential string) (*vaultapi.Client, error) {
	tun := r.tunables(ctx)
	secretmask.Add(credential)
	httpClient, err := guardedClient(cfg.Address, tun.AllowPrivate, tun.Timeout)
	if err != nil {
		return nil, err
	}
	client, err := vaultapi.NewClient(&vaultapi.Config{Address: cfg.Address, HttpClient: httpClient})
	if err != nil {
		return nil, fmt.Errorf("vault: build client: %w", err)
	}
	if cfg.Namespace != "" {
		client.SetNamespace(cfg.Namespace)
	}
	token, err := r.token(ctx, client, cfg, credential, fingerprint(cfg, credential), tun)
	if err != nil {
		return nil, err
	}
	client.SetToken(token)
	return client, nil
}
