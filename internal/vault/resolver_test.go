package vault

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeVaultServer serves the two real HTTP endpoints Resolve needs
// against a genuine net/http/httptest server, rather than mocking the
// vault/api Go client itself: the client has no interface seam to fake,
// so exercising its real HTTP wire behavior is the only way to test this
// package without a live Vault instance.
func fakeVaultServer(t *testing.T, wantToken string, kvData map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/secret/data/myapp/config", func(w http.ResponseWriter, r *http.Request) {
		if wantToken != "" && r.Header.Get("X-Vault-Token") != wantToken {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"data":     kvData,
				"metadata": map[string]any{"version": 1},
			},
		})
	})
	mux.HandleFunc("/v1/auth/approle/login", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"auth": map[string]any{"client_token": "approle-issued-token"}, //nolint:gosec // test fixture, not a real credential
		})
	})
	return httptest.NewServer(mux)
}

func TestResolver_Resolve_TokenAuth(t *testing.T) {
	srv := fakeVaultServer(t, "my-token", map[string]any{"api_key": "s3cr3t"})
	defer srv.Close()

	r := NewResolver()
	cfg := store.VaultSettings{
		Address:    srv.URL,
		AuthMethod: store.VaultAuthMethodToken,
		MountPath:  "secret",
	}
	got, err := r.Resolve(context.Background(), cfg, "my-token", "myapp/config", "api_key")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "s3cr3t" {
		t.Errorf("Resolve() = %q, want %q", got, "s3cr3t")
	}
}

func TestResolver_Resolve_AppRoleAuth(t *testing.T) {
	srv := fakeVaultServer(t, "approle-issued-token", map[string]any{"api_key": "role-secret"})
	defer srv.Close()

	r := NewResolver()
	cfg := store.VaultSettings{
		Address:    srv.URL,
		AuthMethod: store.VaultAuthMethodAppRole,
		RoleID:     "role-123",
		MountPath:  "secret",
	}
	got, err := r.Resolve(context.Background(), cfg, "some-secret-id", "myapp/config", "api_key")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "role-secret" {
		t.Errorf("Resolve() = %q, want %q", got, "role-secret")
	}
}

func TestResolver_Resolve_DefaultMountPath(t *testing.T) {
	srv := fakeVaultServer(t, "my-token", map[string]any{"api_key": "default-mount"})
	defer srv.Close()

	r := NewResolver()
	cfg := store.VaultSettings{
		Address:    srv.URL,
		AuthMethod: store.VaultAuthMethodToken,
		// MountPath left empty: must default to "secret".
	}
	got, err := r.Resolve(context.Background(), cfg, "my-token", "myapp/config", "api_key")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "default-mount" {
		t.Errorf("Resolve() = %q, want %q", got, "default-mount")
	}
}

func TestResolver_Resolve_MissingKey(t *testing.T) {
	srv := fakeVaultServer(t, "my-token", map[string]any{"other_field": "x"})
	defer srv.Close()

	r := NewResolver()
	cfg := store.VaultSettings{Address: srv.URL, AuthMethod: store.VaultAuthMethodToken, MountPath: "secret"}
	if _, err := r.Resolve(context.Background(), cfg, "my-token", "myapp/config", "api_key"); err == nil {
		t.Fatal("Resolve() error = nil, want an error for a missing field")
	}
}

func TestResolver_Resolve_NonStringValue(t *testing.T) {
	srv := fakeVaultServer(t, "my-token", map[string]any{"api_key": 12345})
	defer srv.Close()

	r := NewResolver()
	cfg := store.VaultSettings{Address: srv.URL, AuthMethod: store.VaultAuthMethodToken, MountPath: "secret"}
	if _, err := r.Resolve(context.Background(), cfg, "my-token", "myapp/config", "api_key"); err == nil {
		t.Fatal("Resolve() error = nil, want an error for a non-string field")
	}
}

func TestResolver_Resolve_WrongTokenRejected(t *testing.T) {
	srv := fakeVaultServer(t, "correct-token", map[string]any{"api_key": "s3cr3t"})
	defer srv.Close()

	r := NewResolver()
	cfg := store.VaultSettings{Address: srv.URL, AuthMethod: store.VaultAuthMethodToken, MountPath: "secret"}
	if _, err := r.Resolve(context.Background(), cfg, "wrong-token", "myapp/config", "api_key"); err == nil {
		t.Fatal("Resolve() error = nil, want an error for a rejected token (unreachable/auth-failure case)")
	}
}

func TestResolver_Resolve_UnreachableServer(t *testing.T) {
	r := NewResolver()
	cfg := store.VaultSettings{Address: "http://127.0.0.1:1", AuthMethod: store.VaultAuthMethodToken, MountPath: "secret"}
	if _, err := r.Resolve(context.Background(), cfg, "my-token", "myapp/config", "api_key"); err == nil {
		t.Fatal("Resolve() error = nil, want an error when Vault is unreachable")
	}
}
