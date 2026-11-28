package secrets

import (
	"errors"
	"testing"
)

func TestAppNamespaceRefusesReserved(t *testing.T) {
	for _, n := range []string{"vault", "builtin-registry", "email-settings", "ai-assistant-provider-x", "dns-hetzner"} {
		if err := AppNamespace(n); !errors.Is(err, ErrReservedNamespace) {
			t.Errorf("AppNamespace(%q) = %v, want ErrReservedNamespace", n, err)
		}
	}
	for _, n := range []string{"web", "foo-bar", ControlPlaneService("vault")} {
		if err := AppNamespace(n); err != nil {
			t.Errorf("AppNamespace(%q) = %v, want nil", n, err)
		}
	}
}
