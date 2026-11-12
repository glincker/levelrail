package secretconn

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	vaultapi "github.com/hashicorp/vault/api"

	"github.com/GLINCKER/levelrail/internal/secretprovider"
	"github.com/GLINCKER/levelrail/internal/secretsource"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/vault"
)

// Prober is the optional read-only health check of a provider. It must not
// read a secret value and must not return one.
type Prober interface {
	Probe(ctx context.Context) error
}

// PathProber is the optional read-only check that a credential can reach one path.
type PathProber interface {
	ProbePath(ctx context.Context, path string) error
}

// vaultLoad returns the live Vault settings and credentials for one connection.
type vaultLoad func(ctx context.Context) (cfg store.VaultSettings, current, previous string, graceUntil time.Time, err error)

type vaultProvider struct {
	name string
	load vaultLoad
	res  *vault.Resolver
	now  func() time.Time
}

func (p *vaultProvider) Name() string { return p.name }

// Capabilities: the Vault client reads the latest KV v2 version only.
func (p *vaultProvider) Capabilities() secretsource.Capabilities { return secretsource.Capabilities{} }

func (p *vaultProvider) Resolve(ctx context.Context, ref secretsource.Ref) (secretsource.Value, error) {
	if ref.Version > 0 {
		return secretsource.Value{}, fmt.Errorf("%w: vault connection %s reads the latest version only", secretsource.ErrUnsupported, p.name)
	}
	cfg, cur, prev, grace, err := p.load(ctx)
	if err != nil {
		return secretsource.Value{}, fmt.Errorf("%w: vault connection %s credential unreadable", secretsource.ErrUnavailable, p.name)
	}
	v, err := vault.ResolveWithGrace(p.now(), grace, cur, prev, func(cred string) (string, error) {
		return p.res.Resolve(ctx, cfg, cred, ref.Path, ref.Key)
	})
	if err != nil {
		return secretsource.Value{}, vaultError(p.name, err)
	}
	return secretsource.NewValue([]byte(v), ""), nil
}

// Probe checks the address and credential without reading a secret.
func (p *vaultProvider) Probe(ctx context.Context) error {
	cfg, cur, _, _, err := p.load(ctx)
	if err != nil {
		return fmt.Errorf("%w: credential unreadable", secretsource.ErrUnavailable)
	}
	if err := p.res.Probe(ctx, cfg, cur); err != nil {
		return vaultError(p.name, err)
	}
	return nil
}

// ProbePath checks the credential can read path, without reading its data.
func (p *vaultProvider) ProbePath(ctx context.Context, path string) error {
	cfg, cur, _, _, err := p.load(ctx)
	if err != nil {
		return fmt.Errorf("%w: credential unreadable", secretsource.ErrUnavailable)
	}
	if err := p.res.ProbePath(ctx, cfg, cur, path); err != nil {
		return vaultError(p.name, err)
	}
	return nil
}

// vaultError maps to one sentinel and never echoes the server's error bodies.
func vaultError(name string, err error) error {
	var re *vaultapi.ResponseError
	switch {
	case errors.Is(err, vault.ErrPathDenied):
		return fmt.Errorf("%w: vault connection %s policy denies the path", secretsource.ErrDenied, name)
	case errors.Is(err, vault.ErrSecretMissing):
		return fmt.Errorf("%w: vault connection %s", secretsource.ErrNotFound, name)
	case errors.As(err, &re):
		return statusError(name, "vault", re.StatusCode)
	case errors.Is(err, vault.ErrBlockedAddress), errors.Is(err, vault.ErrRedirectBlocked):
		return fmt.Errorf("%w: vault connection %s address refused by the connection guard", secretsource.ErrDenied, name)
	}
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &ne) {
		return fmt.Errorf("%w: vault connection %s unreachable", secretsource.ErrUnavailable, name)
	}
	return fmt.Errorf("%w: vault connection %s request failed", secretsource.ErrUnavailable, name)
}

func statusError(name, typ string, code int) error {
	switch code {
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s connection %s (HTTP 404)", secretsource.ErrNotFound, typ, name)
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: %s connection %s (HTTP %d)", secretsource.ErrDenied, typ, name, code)
	default:
		return fmt.Errorf("%w: %s connection %s (HTTP %d)", secretsource.ErrUnavailable, typ, name, code)
	}
}

// providerLoad returns the live config and credentials for one provider connection.
type providerLoad func(ctx context.Context) (cfg secretprovider.Config, current, previous string, err error)

type providerAdapter struct {
	name string
	typ  string
	load providerLoad
	res  *secretprovider.Resolver
	tun  func(context.Context) secretprovider.Tunables
	now  func() time.Time
}

func (p *providerAdapter) Name() string { return p.name }

func (p *providerAdapter) Capabilities() secretsource.Capabilities {
	return secretsource.Capabilities{}
}

func (p *providerAdapter) Resolve(ctx context.Context, ref secretsource.Ref) (secretsource.Value, error) {
	if ref.Version > 0 {
		return secretsource.Value{}, fmt.Errorf("%w: %s connection %s reads the latest version only", secretsource.ErrUnsupported, p.typ, p.name)
	}
	pref, err := p.providerRef(ref)
	if err != nil {
		return secretsource.Value{}, err
	}
	cfg, cur, prev, err := p.load(ctx)
	if err != nil {
		return secretsource.Value{}, fmt.Errorf("%w: %s connection %s credential unreadable", secretsource.ErrUnavailable, p.typ, p.name)
	}
	t := p.tun(ctx)
	v, err := vault.ResolveWithGrace(p.now(), cfg.GraceUntil, cur, prev, func(token string) (string, error) {
		return p.res.Fetch(ctx, cfg, token, pref, t)
	})
	if err != nil {
		return secretsource.Value{}, providerError(p.name, p.typ, err)
	}
	return secretsource.NewValue([]byte(v), ""), nil
}

// providerRef maps path and key: Infisical folder and secret, 1Password [vault/]item and field.
// Doppler scopes by project and config, so its path is only a label.
func (p *providerAdapter) providerRef(ref secretsource.Ref) (secretprovider.Ref, error) {
	out := secretprovider.Ref{Type: p.typ, Provider: p.name, Name: ref.Key}
	switch p.typ {
	case secretprovider.TypeInfisical:
		out.Path = "/" + ref.Path
	case secretprovider.TypeOnePassword:
		parts := strings.Split(ref.Path, "/")
		switch len(parts) {
		case 1:
			out.Item = parts[0]
		case 2:
			out.Vault, out.Item = parts[0], parts[1]
		default:
			return out, fmt.Errorf("%w: onepassword path must be item or vault/item", secretsource.ErrInvalidRef)
		}
	}
	return out, nil
}

// Probe lists secret names in scope; no value is read into the result.
func (p *providerAdapter) Probe(ctx context.Context) error {
	cfg, cur, _, err := p.load(ctx)
	if err != nil {
		return fmt.Errorf("%w: credential unreadable", secretsource.ErrUnavailable)
	}
	if err := p.res.Test(ctx, cfg, cur, p.tun(ctx)); err != nil {
		return providerError(p.name, p.typ, err)
	}
	return nil
}

func providerError(name, typ string, err error) error {
	switch {
	case errors.Is(err, secretprovider.ErrNotFound):
		return fmt.Errorf("%w: %s connection %s", secretsource.ErrNotFound, typ, name)
	case errors.Is(err, secretprovider.ErrUnauthorized):
		return fmt.Errorf("%w: %s connection %s rejected the credential", secretsource.ErrDenied, typ, name)
	case errors.Is(err, secretprovider.ErrBlockedAddress), errors.Is(err, secretprovider.ErrRedirectBlocked):
		return fmt.Errorf("%w: %s connection %s address refused by the connection guard", secretsource.ErrDenied, typ, name)
	case errors.Is(err, secretprovider.ErrUnavailable), errors.Is(err, secretprovider.ErrRateLimited):
		return fmt.Errorf("%w: %s connection %s", secretsource.ErrUnavailable, typ, name)
	}
	// Remaining errors come from this repo's own checks (path limits, empty value) and carry no provider body.
	return fmt.Errorf("%w: %s connection %s: %s", secretsource.ErrUnavailable, typ, name, err.Error())
}
