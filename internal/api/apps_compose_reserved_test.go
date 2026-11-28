package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secrets"
)

func composeMagicYAML(service string) string {
	return "services:\n  " + service + ":\n    image: x:1\n    environment:\n      password: $SERVICE_PASSWORD_A\n"
}

func TestComposeDeployRefusesReservedNamespaceSecrets(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	sm := secrets.NewManager(db, mk)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithComposeSecrets(sm))
	cookie := loginTestSession(t, rt, db)

	cases := []struct{ app, service, ns string }{
		{"builtin", "registry", "builtin-registry"},
		{"email", "settings", "email-settings"},
		{"cloudflare", "tunnel", "cloudflare-tunnel"},
		{"ai", "assistant", "ai-assistant"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/"+c.app+"/compose", composeMagicYAML(c.service)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400; body = %s", c.ns, rec.Code, rec.Body.String())
		}
		if _, err := sm.Resolve(context.Background(), c.ns, "password"); !errors.Is(err, secrets.ErrValueNotFound) {
			t.Errorf("%s: secret row exists after refused deploy (err = %v)", c.ns, err)
		}
	}
}

func TestComposeDeployOwnerRedeployStillWorks(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	sm := secrets.NewManager(db, mk)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithComposeSecrets(sm))
	cookie := loginTestSession(t, rt, db)
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/foo/compose", composeMagicYAML("bar")))
		if rec.Code != http.StatusOK {
			t.Fatalf("deploy %d: status = %d; body = %s", i, rec.Code, rec.Body.String())
		}
	}
	if _, err := sm.Resolve(context.Background(), "foo-bar", "password"); err != nil {
		t.Errorf("owner secret missing: %v", err)
	}
}
