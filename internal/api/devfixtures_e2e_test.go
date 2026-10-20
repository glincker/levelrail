package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// This file is a regression test that dev-fixtures.yml's documented
// ability matrix still matches what internal/api/router.go actually
// enforces. It is deliberately not a general-purpose ability test:
// internal/api/tokens_test.go and internal/api/abilities_test.go already
// cover requireAbility and hasAbility in the general case with synthetic
// tokens. This file instead reads the real dev-fixtures.yml at the repo
// root, seeds it into a fresh router through the same seedDevFixtures
// core MaybeSeedDevFixturesFromFile calls in production, and fires real
// HTTP requests at real routes. If a future change to router.go's
// requireAbility wiring silently breaks a promise dev-fixtures.yml's own
// header comment makes (e.g. "a read token gets 200 on GET /apps"),
// this test fails in CI instead of a developer discovering it by hand
// with curl.

// realDevFixturesPath is relative to this package's directory
// (internal/api), which is where `go test` sets its working directory
// regardless of the directory the test command itself was invoked from.
const realDevFixturesPath = "../../dev-fixtures.yml"

// loadRealDevFixtures reads and parses the actual dev-fixtures.yml
// shipped at the repo root, via the same os.ReadFile + ParseDevFixtures
// pair MaybeSeedDevFixturesFromFile uses internally. If that file goes
// missing or becomes malformed, this fails loudly here rather than only
// at server startup.
func loadRealDevFixtures(t *testing.T) *DevFixtures {
	t.Helper()
	data, err := os.ReadFile(realDevFixturesPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v (dev-fixtures.yml must exist at the repo root)", realDevFixturesPath, err)
	}
	fixtures, err := ParseDevFixtures(data)
	if err != nil {
		t.Fatalf("ParseDevFixtures(dev-fixtures.yml) error = %v", err)
	}
	return fixtures
}

// fixturePlaintext looks up a fixture token's plaintext by its `name`
// field rather than hardcoding the plaintext string in this file, so
// this test is tied to the real file's content instead of a second copy
// of it drifting out of sync.
func fixturePlaintext(t *testing.T, fixtures *DevFixtures, name string) string {
	t.Helper()
	for _, tok := range fixtures.Tokens {
		if tok.Name == name {
			return tok.Plaintext
		}
	}
	t.Fatalf("dev-fixtures.yml has no token named %q", name)
	return ""
}

// newDevFixturesTestRouter builds a fresh router and store, then seeds
// it with the real dev-fixtures.yml tokens via seedDevFixtures, the same
// unexported core MaybeSeedDevFixturesFromFile calls after reading and
// parsing the file. enabled is passed as true directly, mirroring how
// MaybeSeedDevFixturesFromFile only ever calls it that way once
// devModeEnabled() and a readable file are already established; the
// devModeEnabled() gate itself is exercised separately by
// devmode_debug_test.go / devmode_release_test.go, not here.
func newDevFixturesTestRouter(t *testing.T, fixtures *DevFixtures) (*Router, *store.DB) {
	t.Helper()
	rt, db := newTestRouter(t)
	if err := seedDevFixtures(context.Background(), db, rt.logger, fixtures, true); err != nil {
		t.Fatalf("seedDevFixtures() error = %v", err)
	}
	return rt, db
}

func bearerRequest(method, target, body, plaintext string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	req.Header.Set("Authorization", "Bearer "+plaintext)
	return req
}

func TestDevFixturesE2E_ReadToken_CanListButNotCreate(t *testing.T) {
	fixtures := loadRealDevFixtures(t)
	rt, _ := newDevFixturesTestRouter(t, fixtures)
	token := fixturePlaintext(t, fixtures, "dev-read")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodGet, "/api/v1/apps", "", token))
	if rec.Code != http.StatusOK {
		t.Errorf("GET /apps with dev-read token: status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodPost, "/api/v1/apps", `{"name":"web","image":"nginx:alpine","port":80}`, token))
	if rec.Code != http.StatusForbidden {
		t.Errorf("POST /apps with dev-read token: status = %d, want %d: a read-scoped token must not be able to create apps", rec.Code, http.StatusForbidden)
	}
}

func TestDevFixturesE2E_ReadSensitiveToken_DoesNotImplyPlainRead(t *testing.T) {
	// No route in router.go requires read:sensitive on its own today
	// (see the PUT secrets route, which is write:sensitive, not
	// read:sensitive). The one thing worth pinning down here is that
	// read:sensitive is its own distinct ability and grants nothing on a
	// route that requires plain read, the same "abilities don't imply
	// each other except via root" property the deploy case below checks.
	fixtures := loadRealDevFixtures(t)
	rt, _ := newDevFixturesTestRouter(t, fixtures)
	token := fixturePlaintext(t, fixtures, "dev-read-sensitive")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodGet, "/api/v1/apps", "", token))
	if rec.Code != http.StatusForbidden {
		t.Errorf("GET /apps with dev-read-sensitive token: status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestDevFixturesE2E_WriteToken_CanCreateButNotSetSecret(t *testing.T) {
	fixtures := loadRealDevFixtures(t)
	rt, _ := newDevFixturesTestRouter(t, fixtures)
	token := fixturePlaintext(t, fixtures, "dev-write")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodPost, "/api/v1/apps", `{"name":"web","image":"nginx:alpine","port":80}`, token))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /apps with dev-write token: status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodPut, "/api/v1/apps/web/secrets/API_KEY", `{"value":"sk-abc"}`, token))
	if rec.Code != http.StatusForbidden {
		t.Errorf("PUT secrets with dev-write token: status = %d, want %d: a plain write-scoped token must not be able to set secret values", rec.Code, http.StatusForbidden)
	}
}

func TestDevFixturesE2E_WriteSensitiveToken_ReachesSecretsHandler(t *testing.T) {
	fixtures := loadRealDevFixtures(t)
	rt, db := newDevFixturesTestRouter(t, fixtures)
	token := fixturePlaintext(t, fixtures, "dev-write-sensitive")

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodPut, "/api/v1/apps/web/secrets/API_KEY", `{"value":"sk-abc"}`, token))
	// This router is built via newTestRouter/newDevFixturesTestRouter
	// with no WithSecretSetter configured, the same "no setter
	// configured" case internal/api/secrets_test.go's
	// TestHandleSetSecret_NoSetterConfigured exercises: handleSetSecret
	// returns 501 once it gets past the ability check and finds no
	// secrets backend wired up. A 501 here is proof the write:sensitive
	// ability check passed and the request reached the handler; it is
	// not a bug, and it is not the same as a 403 an under-scoped token
	// would get.
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("PUT secrets with dev-write-sensitive token: status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestDevFixturesE2E_DeployToken_CannotRead(t *testing.T) {
	// Deploy does not imply read: this is the ability pairing people get
	// wrong most often, worth its own explicit assertion rather than
	// folding into another test.
	fixtures := loadRealDevFixtures(t)
	rt, _ := newDevFixturesTestRouter(t, fixtures)
	token := fixturePlaintext(t, fixtures, "dev-deploy")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodGet, "/api/v1/apps", "", token))
	if rec.Code != http.StatusForbidden {
		t.Errorf("GET /apps with dev-deploy token: status = %d, want %d: a deploy-only token must not be able to read apps", rec.Code, http.StatusForbidden)
	}
}

func TestDevFixturesE2E_RootToken_CanReadAnything(t *testing.T) {
	fixtures := loadRealDevFixtures(t)
	rt, _ := newDevFixturesTestRouter(t, fixtures)
	token := fixturePlaintext(t, fixtures, "dev-root")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodGet, "/api/v1/apps", "", token))
	if rec.Code != http.StatusOK {
		t.Errorf("GET /apps with dev-root token: status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestDevFixturesE2E_NoCredential_Unauthorized(t *testing.T) {
	fixtures := loadRealDevFixtures(t)
	rt, _ := newDevFixturesTestRouter(t, fixtures)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /apps with no credential: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
