package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// proxyLabel records the applied pull-through config so a change replaces the container;
// no label means a normal registry, so existing containers are untouched.
const proxyLabel = "levelrail.registry.proxy"

// applyProxy adds the pull-through cache env to spec when an upstream is configured and
// returns the fingerprint that goes on the container label (empty when not in proxy mode).
func (c *Controller) applyProxy(ctx context.Context, settings store.RegistrySettings, spec *docker.ContainerSpec) (string, error) {
	if settings.ProxyRemoteURL == "" {
		return "", nil
	}
	spec.Env["REGISTRY_PROXY_REMOTEURL"] = settings.ProxyRemoteURL
	password := ""
	if settings.ProxyUsername != "" {
		spec.Env["REGISTRY_PROXY_USERNAME"] = settings.ProxyUsername
		ok, err := c.creds.Exists(ctx, store.RegistrySettingsSecretsKey(), store.RegistryProxyPasswordEnvKey)
		if err != nil {
			return "", fmt.Errorf("check proxy password: %w", err)
		}
		if ok {
			password, err = c.creds.Resolve(ctx, store.RegistrySettingsSecretsKey(), store.RegistryProxyPasswordEnvKey)
			if err != nil {
				return "", fmt.Errorf("resolve proxy password: %w", err)
			}
			spec.Env["REGISTRY_PROXY_PASSWORD"] = password
		}
	}
	return proxyFingerprint(settings.ProxyRemoteURL, settings.ProxyUsername, password), nil
}

func proxyFingerprint(remote, user, password string) string {
	sum := sha256.Sum256([]byte(remote + "\x00" + user + "\x00" + password))
	return "proxy:" + hex.EncodeToString(sum[:6])
}
