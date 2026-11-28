package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/envrules"
)

type envFixtureCase struct {
	Name       string            `json:"name"`
	Set        map[string]string `json:"set"`
	Existing   envrules.Existing `json:"existing"`
	Policy     json.RawMessage   `json:"policy"`
	Options    envrules.Options  `json:"options"`
	WantErrors []string          `json:"want_errors"`
}

// The handler must accept and refuse exactly what the shared fixture says,
// under the default policy, so API, CLI and web agree.
func TestEnvPutFollowsSharedFixture(t *testing.T) {
	raw, err := os.ReadFile("../envrules/testdata/envrules.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Cases []envFixtureCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	ran := 0
	for _, c := range f.Cases {
		if len(c.Policy) > 0 || len(c.Existing.Names) > 0 || c.Options.Secret || c.Options.ExistingUnknown {
			continue
		}
		wantRefused := len(c.WantErrors) > 0
		if len(c.WantErrors) == 1 && c.WantErrors[0] == "crlf" {
			wantRefused = false // JSON bodies are byte exact, so the API keeps the CR
		}
		ran++
		t.Run(c.Name, func(t *testing.T) {
			rt, db := newTestRouterWithSecrets(t, &fakeSecretSetter{})
			cookie := loginTestSession(t, rt, db)
			seedDualCopyApp(t, db, c.Existing.Plain)
			body, _ := json.Marshal(map[string]map[string]string{"set": c.Set})
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/env", string(body)))
			if wantRefused && rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
			}
			if !wantRefused && rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
	if ran < 35 {
		t.Fatalf("only %d fixture cases ran", ran)
	}
}

func TestSecretSetRefusesBadKeysAndKeepsWarningsOnEnvPut(t *testing.T) {
	rt, db := newTestRouterWithSecrets(t, &fakeSecretSetter{})
	cookie := loginTestSession(t, rt, db)
	seedDualCopyApp(t, db, nil)
	for _, key := range []string{"X_PREVIOUS", "a%20b", "LD_PRELOAD", "LEVELRAIL_X"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/secrets/"+key, `{"value":"v"}`))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("secret key %q: status = %d, want 400, body = %s", key, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/env", `{"set":{"A":" pad"}}`))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"code":"whitespace"`) {
		t.Fatalf("want 200 with a whitespace warning, got %d %s", rec.Code, rec.Body.String())
	}
}
