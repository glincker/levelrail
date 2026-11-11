package application

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/vault"
)

// A Vault value and credential must reach only the container env: never logs, conditions or the stored desired state.
func TestVaultCanaryNeverLeaksFromResolvePath(t *testing.T) {
	//nolint:gosec // fake canary values
	const valueCanary, credCanary = "CANARY_7f3a_value_9d41c0de", "CANARY_7f3a_credential_5b2e77aa"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != credCanary {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"data": map[string]any{"api_key": valueCanary}, "metadata": map[string]any{}}})
	}))
	defer srv.Close()

	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(secretmask.Handler(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	t.Cleanup(func() { slog.SetDefault(prev) })

	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		VaultEnv: map[string]store.VaultEnvRef{"API_KEY": {Path: "app", Key: "api_key"}},
	}
	resolver := vault.NewResolverWith(func(context.Context) vault.Tunables {
		return vault.Tunables{Timeout: 5 * time.Second, AllowPrivate: true, RenewPercent: 80}
	})
	build := func(cred string) (*Controller, *fakeRuntime) {
		rt := newFakeRuntime(0)
		settings := &fakeVaultSettingsStore{settings: store.VaultSettings{Enabled: true, Address: srv.URL, AuthMethod: store.VaultAuthMethodToken, MountPath: "secret"}}
		secrets := newFakeSecretResolver(map[string]string{"vault/credential": cred})
		return New("web", &fakeStore{svc: desired}, rt, WithVaultSettings(settings), WithVaultResolver(resolver), WithSecretResolver(secrets)), rt
	}

	var surfaces []string
	for _, cred := range []string{credCanary, credCanary + "_wrong"} {
		c, rt := build(cred)
		result, err := c.Reconcile(context.Background())
		if cred == credCanary {
			if err != nil || rt.lastCreateEnv["API_KEY"] != valueCanary {
				t.Fatalf("the happy path must hand the value to the container only, err=%v", err)
			}
		} else if err == nil {
			t.Fatal("a rejected credential must fail the pass")
		} else {
			surfaces = append(surfaces, err.Error())
		}
		cond, _ := json.Marshal(result)
		surfaces = append(surfaces, string(cond))
	}
	stored, _ := json.Marshal(desired) // what a revision would persist
	surfaces = append(surfaces, string(stored), secretmask.Mask("echo "+valueCanary+" "+credCanary), logs.String())
	for i, s := range surfaces {
		if strings.Contains(s, valueCanary) || strings.Contains(s, credCanary) {
			t.Fatalf("canary leaked on surface %d: %.80s", i, s)
		}
	}
}
