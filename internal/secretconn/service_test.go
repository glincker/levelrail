package secretconn

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secretprovider"
	"github.com/GLINCKER/levelrail/internal/secretsource"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/vault"
)

type memSecrets struct{ vals map[string]string }

func (m *memSecrets) k(o, k string) string { return o + "/" + k }
func (m *memSecrets) SetValue(_ context.Context, o, k, v string) error {
	m.vals[m.k(o, k)] = v
	return nil
}
func (m *memSecrets) Resolve(_ context.Context, o, k string) (string, error) {
	v, ok := m.vals[m.k(o, k)]
	if !ok {
		return "", errors.New("missing")
	}
	return v, nil
}
func (m *memSecrets) Exists(_ context.Context, o, k string) (bool, error) {
	_, ok := m.vals[m.k(o, k)]
	return ok, nil
}
func (m *memSecrets) ListKeys(_ context.Context, o string) ([]store.SecretKeyInfo, error) {
	var out []store.SecretKeyInfo
	for k := range m.vals {
		if key, ok := strings.CutPrefix(k, o+"/"); ok {
			out = append(out, store.SecretKeyInfo{Key: key})
		}
	}
	return out, nil
}
func (m *memSecrets) DeleteValue(_ context.Context, o, k string) error {
	delete(m.vals, m.k(o, k))
	return nil
}

type memRows struct {
	rows map[string]store.SecretConnection
}

func (m *memRows) ListSecretConnections(context.Context) ([]store.SecretConnection, error) {
	var out []store.SecretConnection
	for _, r := range m.rows {
		out = append(out, r)
	}
	return out, nil
}
func (m *memRows) GetSecretConnection(_ context.Context, n string) (store.SecretConnection, error) {
	r, ok := m.rows[n]
	if !ok {
		return r, store.ErrSecretConnectionNotFound
	}
	return r, nil
}
func (m *memRows) CreateSecretConnection(_ context.Context, c store.SecretConnection) error {
	if _, ok := m.rows[c.Name]; ok {
		return store.ErrSecretConnectionExists
	}
	m.rows[c.Name] = c
	return nil
}
func (m *memRows) UpdateSecretConnection(_ context.Context, c store.SecretConnection) error {
	r := m.rows[c.Name]
	r.Config, r.CredentialRef, r.Policy = c.Config, c.CredentialRef, c.Policy
	m.rows[c.Name] = r
	return nil
}
func (m *memRows) DeleteSecretConnection(_ context.Context, n string) error {
	delete(m.rows, n)
	return nil
}

type vaultSettings struct{ s store.VaultSettings }

func (v vaultSettings) GetVaultSettings(context.Context) (store.VaultSettings, error) {
	return v.s, nil
}

const credCanary = "cred-canary-4471" //nolint:gosec // fixture

func newSvc(vs store.VaultSettings) (*Service, *memSecrets) {
	sec := &memSecrets{vals: map[string]string{}}
	return &Service{
		Rows: &memRows{rows: map[string]store.SecretConnection{}}, Secrets: sec, Vault: vaultSettings{vs},
		Sources: secretsource.NewResolver(), VaultRes: vault.NewResolver(), ProvRes: secretprovider.NewResolver(),
		ProvTun: func(context.Context) secretprovider.Tunables {
			return secretprovider.Tunables{Timeout: 2e9, MaxResponseBytes: 1 << 20}
		},
	}, sec
}

func fakeVault(t *testing.T, status int) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != credCanary {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if strings.HasSuffix(r.URL.Path, "lookup-self") {
			_, _ = w.Write([]byte(`{"data":{"id":"x"}}`))
			return
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"errors":["body-leak-marker"]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"data":{"k":"s3cret-val"},"metadata":{"version":3}}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLegacyVaultMapsToConnectionVaultAndResolves(t *testing.T) {
	srv := fakeVault(t, http.StatusOK)
	svc, sec := newSvc(store.VaultSettings{Enabled: true, Address: srv.URL, AuthMethod: "token", MountPath: "secret"})
	sec.vals[store.VaultSecretsKey()+"/"+store.VaultCredentialEnvKey] = credCanary
	ctx := context.Background()
	if err := svc.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	c, err := svc.Get(ctx, "vault")
	if err != nil || !c.Legacy || !c.HasCredential {
		t.Fatalf("legacy vault = %+v, %v", c, err)
	}
	v, err := svc.Sources.Resolve(ctx, secretsource.Ref{Conn: "vault", Path: "apps/web", Key: "k"})
	if err != nil || string(v.Reveal()) != "s3cret-val" {
		t.Fatalf("resolve = %v", err)
	}
}

func TestLegacyProviderKeepsNameAndIsRegistered(t *testing.T) {
	svc, sec := newSvc(store.VaultSettings{})
	ctx := context.Background()
	st := secretprovider.Store{Secrets: sec}
	if _, err := st.Put(ctx, secretprovider.Config{Name: "dp", Type: secretprovider.TypeDoppler, Project: "p", Environment: "e"}, "tok", 0, svc.now()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.Sources.Provider("dp"); !ok {
		t.Fatal("doppler provider not registered under its own name")
	}
	if err := svc.Delete(ctx, "dp"); !errors.Is(err, ErrLegacy) {
		t.Fatalf("delete legacy err = %v", err)
	}
}

func TestCreateValidationAndConflicts(t *testing.T) {
	svc, _ := newSvc(store.VaultSettings{Enabled: true, Address: "https://v", AuthMethod: "token"})
	ctx := context.Background()
	good := Input{Type: TypeVault, Config: Config{Address: "https://v:8200"}, Credential: "t"}
	cases := map[string]struct {
		name string
		in   Input
		want error
	}{
		"bad name":      {"Prod", good, ErrInvalid},
		"bad type":      {"a", Input{Type: "x", Credential: "t"}, ErrInvalid},
		"bad address":   {"a", Input{Type: TypeVault, Config: Config{Address: "ftp://x"}, Credential: "t"}, ErrInvalid},
		"approle role":  {"a", Input{Type: TypeVault, Config: Config{Address: "https://v", AuthMethod: "approle"}, Credential: "t"}, ErrInvalid},
		"no credential": {"a", Input{Type: TypeVault, Config: Config{Address: "https://v"}}, ErrInvalid},
		"both":          {"a", Input{Type: TypeVault, Config: Config{Address: "https://v"}, Credential: "t", CredentialRef: "x/y"}, ErrInvalid},
		"legacy name":   {"vault", good, ErrConflict},
		"missing ref":   {"a", Input{Type: TypeVault, Config: Config{Address: "https://v"}, CredentialRef: "x/y"}, ErrInvalid},
		"op needs url":  {"a", Input{Type: TypeOnePassword, Credential: "t"}, ErrInvalid},
	}
	for n, c := range cases {
		if _, err := svc.Create(ctx, "u", c.name, c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", n, err, c.want)
		}
	}
	if _, err := svc.Create(ctx, "u", "prod-vault", good); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "u", "prod-vault", good); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate err = %v", err)
	}
}

func TestUpdateGuardsCredentialAndDeleteUnregisters(t *testing.T) {
	svc, sec := newSvc(store.VaultSettings{})
	ctx := context.Background()
	if _, err := svc.Create(ctx, "u", "pv", Input{Type: TypeVault, Config: Config{Address: "https://v:8200"}, Credential: credCanary}); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.Sources.Provider("pv"); !ok {
		t.Fatal("not registered after create")
	}
	if _, err := svc.Update(ctx, "pv", Input{Config: Config{Address: "https://other:8200"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("address change without credential err = %v", err)
	}
	if _, err := svc.Update(ctx, "pv", Input{Type: TypeDoppler, Config: Config{Address: "https://v:8200"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("type change err = %v", err)
	}
	if err := svc.Delete(ctx, "pv"); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.Sources.Provider("pv"); ok {
		t.Fatal("still registered after delete")
	}
	if ok, _ := sec.Exists(ctx, SecretsOwner, tokenKey("pv")); ok {
		t.Fatal("stored credential not removed")
	}
}

func TestVaultAdapterErrorMappingNeverEchoesBody(t *testing.T) {
	for status, want := range map[int]error{404: secretsource.ErrNotFound, 403: secretsource.ErrDenied, 503: secretsource.ErrUnavailable} {
		srv := fakeVault(t, status)
		svc, _ := newSvc(store.VaultSettings{})
		ctx := context.Background()
		if _, err := svc.Create(ctx, "u", "pv", Input{Type: TypeVault, Config: Config{Address: srv.URL}, Credential: credCanary}); err != nil {
			t.Fatal(err)
		}
		_, err := svc.Sources.Resolve(ctx, secretsource.Ref{Conn: "pv", Path: "p", Key: "k"})
		if !errors.Is(err, want) {
			t.Errorf("status %d: err = %v, want %v", status, err, want)
		}
		if err != nil && (strings.Contains(err.Error(), "body-leak-marker") || strings.Contains(err.Error(), credCanary)) {
			t.Errorf("status %d: error echoes provider body or credential: %v", status, err)
		}
	}
}

func TestVersionRequestIsUnsupported(t *testing.T) {
	srv := fakeVault(t, 200)
	svc, _ := newSvc(store.VaultSettings{})
	ctx := context.Background()
	_, _ = svc.Create(ctx, "u", "pv", Input{Type: TypeVault, Config: Config{Address: srv.URL}, Credential: credCanary})
	_, err := svc.Sources.Resolve(ctx, secretsource.Ref{Conn: "pv", Path: "p", Key: "k", Version: 2})
	if !errors.Is(err, secretsource.ErrUnsupported) {
		t.Fatalf("err = %v", err)
	}
}

type probeFake struct {
	secretsource.Provider
	err error
}

func (p probeFake) Probe(context.Context) error { return p.err }

func TestTestConnectionUsesProbeAndReportsClassOnly(t *testing.T) {
	svc, _ := newSvc(store.VaultSettings{})
	ctx := context.Background()
	_, _ = svc.Create(ctx, "u", "pv", Input{Type: TypeVault, Config: Config{Address: "https://v"}, Credential: credCanary})
	for _, c := range []struct {
		err   error
		class string
	}{{nil, "ok"}, {secretsource.ErrDenied, "denied"}, {errors.New("boom " + credCanary), "unavailable"}} {
		svc.Build = func(Connection) secretsource.Provider { return probeFake{secretsource.NewFake("pv"), c.err} }
		res, err := svc.Test(ctx, "pv", "")
		if err != nil || res.Class != c.class || res.OK != (c.err == nil) || strings.Contains(res.Reason, credCanary) {
			t.Errorf("probe %v => %+v, %v", c.err, res, err)
		}
	}
}

func TestVaultProbeReadsNoSecret(t *testing.T) {
	srv := fakeVault(t, http.StatusOK)
	svc, _ := newSvc(store.VaultSettings{})
	ctx := context.Background()
	_, _ = svc.Create(ctx, "u", "pv", Input{Type: TypeVault, Config: Config{Address: srv.URL}, Credential: credCanary})
	res, err := svc.Test(ctx, "pv", "")
	if err != nil || !res.OK {
		t.Fatalf("probe = %+v, %v", res, err)
	}
}

func TestCreateRejectsBlockedAddressesNamingRule(t *testing.T) {
	svc, _ := newSvc(store.VaultSettings{})
	for addr, rule := range map[string]string{
		"http://169.254.169.254":          "link-local",
		"http://[fd00:ec2::254]:8200":     "", // private, allowed by default
		"http://0.0.0.0:8200":             "unspecified",
		"http://239.1.1.1":                "multicast",
		"http://metadata.google.internal": "metadata host",
	} {
		_, err := svc.Create(context.Background(), "u", "n"+strings.NewReplacer(":", "", ".", "", "/", "", "[", "", "]", "").Replace(addr)[:6], Input{Type: TypeVault, Config: Config{Address: addr}, Credential: "t"})
		if rule == "" {
			if err != nil {
				t.Errorf("%s: unexpected %v", addr, err)
			}
			continue
		}
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), rule) {
			t.Errorf("%s: err = %v, want invalid naming %q", addr, err, rule)
		}
	}
}

func TestPathProbeReportsDeniedAndNotFoundWithoutValues(t *testing.T) {
	caps := []string{"read"}
	metaStatus := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "lookup-self"):
			_, _ = w.Write([]byte(`{"data":{"id":"x"}}`))
		case strings.HasSuffix(r.URL.Path, "capabilities-self"):
			_, _ = w.Write([]byte(`{"capabilities":["` + strings.Join(caps, `","`) + `"],"data":{"capabilities":["` + strings.Join(caps, `","`) + `"]}}`))
		case strings.Contains(r.URL.Path, "/metadata/"):
			w.WriteHeader(metaStatus)
			if metaStatus == http.StatusOK {
				_, _ = w.Write([]byte(`{"data":{"current_version":1}}`))
			}
		default:
			t.Errorf("unexpected request %s (a value read)", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	svc, _ := newSvc(store.VaultSettings{})
	ctx := context.Background()
	_, _ = svc.Create(ctx, "u", "pv", Input{Type: TypeVault, Config: Config{Address: srv.URL}, Credential: credCanary})
	// The fake vault above does not check the token, so any credential logs in.
	check := func(want string) {
		t.Helper()
		res, err := svc.Test(ctx, "pv", "app1#key")
		if err != nil || res.Class != want {
			t.Fatalf("class = %q (%v), want %q", res.Class, err, want)
		}
	}
	check("ok")
	caps = []string{"deny"}
	check("denied")
	caps, metaStatus = []string{"read"}, http.StatusNotFound
	check("not_found")
	if res, _ := svc.Test(ctx, "pv", ""); res.Class != "ok" {
		t.Fatalf("login-only probe = %+v", res)
	}
}
