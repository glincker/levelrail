package authengine_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/secrets"
)

func TestInstanceOrganizationIsCreatedOnce(t *testing.T) {
	e := newSessEnv(t)
	first := e.eng.OrganizationID()
	if first == "" || first == "00000000000000000000000000" {
		t.Fatalf("organization id = %q", first)
	}
	again, err := authengine.New(e.db.DB, authengine.Config{BaseURL: "http://example.test", Directory: authengine.NewDirectory(e.db.DB)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(again.Close)
	if again.OrganizationID() != first {
		t.Fatalf("second start made a new organization: %s != %s", again.OrganizationID(), first)
	}
	var n int
	if err := e.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM theauth_organizations`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("organizations = %d, %v", n, err)
	}
}

func TestSAMLNeedsAKeypair(t *testing.T) {
	e := newSessEnv(t)
	if e.eng.SAMLEnabled() {
		t.Fatal("SAML is on without an SP keypair")
	}
	kp, err := authengine.GenerateSAMLKeyPair(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	eng, err := authengine.New(e.db.DB, authengine.Config{BaseURL: "http://example.test", Directory: authengine.NewDirectory(e.db.DB), SAML: kp})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(eng.Close)
	if !eng.SAMLEnabled() {
		t.Fatal("SAML stayed off with a keypair")
	}
}

func TestPublicHandlerHidesLibraryOrgAndSAMLRoutes(t *testing.T) {
	e := newSessEnv(t)
	kp, err := authengine.GenerateSAMLKeyPair(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	eng, err := authengine.New(e.db.DB, authengine.Config{BaseURL: "http://example.test", Directory: authengine.NewDirectory(e.db.DB), SAML: kp})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(eng.Close)
	p := eng.Prefix()
	raw := httptest.NewRecorder()
	eng.Auth().Handler().ServeHTTP(raw, httptest.NewRequest(http.MethodGet, p+"/saml/01ARZ3NDEKTSV4RRFFQ69G5FAV/metadata", nil))
	if raw.Code == http.StatusNotFound && raw.Body.Len() == len("404 page not found\n") {
		t.Fatal("positive control: the library itself does not serve SAML routes, so the filter test proves nothing")
	}
	for _, target := range []string{p + "/orgs", p + "/orgs/x/saml/connections", p + "/saml/x/acs", p + "/saml/x/login", p + "/saml/x/metadata"} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			rec := httptest.NewRecorder()
			eng.Handler().ServeHTTP(rec, httptest.NewRequest(method, target, nil))
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s %s = %d, want 404", method, target, rec.Code)
			}
		}
	}
}

func TestSAMLKeyPairIsStoredAndReused(t *testing.T) {
	ks := &memKeyStore{values: map[string]string{}}
	ctx := context.Background()
	first, err := authengine.LoadOrCreateSAMLKeyPair(ctx, ks)
	if err != nil {
		t.Fatal(err)
	}
	second, err := authengine.LoadOrCreateSAMLKeyPair(ctx, ks)
	if err != nil {
		t.Fatal(err)
	}
	if string(first.CertPEM) != string(second.CertPEM) || string(first.KeyPEM) != string(second.KeyPEM) {
		t.Fatal("second load generated a new keypair")
	}
	for k := range ks.values {
		ks.values[k] = "garbage"
	}
	if _, err := authengine.LoadOrCreateSAMLKeyPair(ctx, ks); err == nil {
		t.Fatal("a corrupt stored keypair was accepted")
	}
}

type memKeyStore struct{ values map[string]string }

func (m *memKeyStore) Resolve(_ context.Context, service, key string) (string, error) {
	v, ok := m.values[service+"|"+key]
	if !ok {
		return "", secrets.ErrValueNotFound
	}
	return v, nil
}

func (m *memKeyStore) SetValue(_ context.Context, service, key, plaintext string) error {
	m.values[service+"|"+key] = plaintext
	return nil
}
