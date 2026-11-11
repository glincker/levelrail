package vault

import (
	"context"
	"fmt"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/api/auth/approle"

	"github.com/GLINCKER/levelrail/internal/store"
)

// token returns the Vault token for this read: the credential itself for token
// auth, or a cached AppRole login renewed once its renewal point has passed.
func (r *Resolver) token(ctx context.Context, client *vaultapi.Client, cfg store.VaultSettings, credential, fp string, tun Tunables) (string, error) {
	if cfg.AuthMethod != store.VaultAuthMethodAppRole {
		return credential, nil
	}
	if t, ok := r.liveSession(fp); ok {
		return t, nil
	}
	// One login serves every concurrent caller; a failure is returned to all, never retried silently.
	v, err, _ := r.logins.Do(fp, func() (any, error) {
		if t, ok := r.liveSession(fp); ok {
			return t, nil
		}
		return r.login(ctx, client, cfg, credential, fp, tun)
	})
	if err != nil {
		return "", err
	}
	t, _ := v.(string)
	return t, nil
}

func (r *Resolver) liveSession(fp string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[fp]
	if !ok || (!s.renewAt.IsZero() && !r.now().Before(s.renewAt)) {
		delete(r.sessions, fp)
		return "", false
	}
	return s.token, true
}

func (r *Resolver) dropSession(fp string) {
	r.mu.Lock()
	delete(r.sessions, fp)
	r.mu.Unlock()
}

func (r *Resolver) login(ctx context.Context, client *vaultapi.Client, cfg store.VaultSettings, credential, fp string, tun Tunables) (string, error) {
	auth, err := approle.NewAppRoleAuth(cfg.RoleID, &approle.SecretID{FromString: credential})
	if err != nil {
		return "", fmt.Errorf("vault: build approle auth: %w", err)
	}
	loginSecret, err := client.Auth().Login(ctx, auth)
	if err != nil {
		return "", fmt.Errorf("vault: approle login: %w", err)
	}
	if loginSecret == nil || loginSecret.Auth == nil || loginSecret.Auth.ClientToken == "" {
		return "", fmt.Errorf("vault: approle login returned no client token")
	}
	s := session{token: loginSecret.Auth.ClientToken}
	if lease := time.Duration(loginSecret.Auth.LeaseDuration) * time.Second; lease > 0 {
		s.renewAt = r.now().Add(lease * time.Duration(tun.RenewPercent) / 100)
	}
	r.mu.Lock()
	r.sessions[fp] = s
	r.mu.Unlock()
	return s.token, nil
}
