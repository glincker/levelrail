package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDiffEnvSidesKeysOnly(t *testing.T) {
	base := envDiffSide{plain: map[string]string{"A": "1", "B": "same", "C": "old", "ONLY_BASE": "x"}, opaque: map[string]bool{"S": true, "T": true}}
	other := envDiffSide{plain: map[string]string{"B": "same", "C": "new", "ONLY_OTHER": "y", "T": "plain"}, opaque: map[string]bool{"S": true}}
	res := diffEnvSides("web", "other", base, other)
	if strings.Join(res.Added, ",") != "ONLY_OTHER" || strings.Join(res.Removed, ",") != "A,ONLY_BASE" {
		t.Fatalf("added/removed = %v / %v", res.Added, res.Removed)
	}
	if strings.Join(res.Changed, ",") != "C,T" || res.Same != 1 || res.SecretUnknown != 1 {
		t.Fatalf("changed=%v same=%d secret=%d", res.Changed, res.Same, res.SecretUnknown)
	}
}

func TestHandleEnvDiffNoValueLeak(t *testing.T) {
	f := newEnvFixture(t)
	ctx := context.Background()
	const newVal = "different-plain-value-zzz"
	if err := f.db.SaveDesiredService(ctx, store.DesiredService{Name: "third", Image: "img:v1", Port: 3000, Env: map[string]string{
		"PORT": newVal, "EXTRA": "extra-value-qqq", "API_SECRET": plainSecretValue + "x",
	}}); err != nil {
		t.Fatal(err)
	}
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/third/secrets/DB_PASSWORD", `{"value":"another-stored-secret-value"}`, http.StatusNoContent)

	rec := f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/web/env-diff?against=third", "", http.StatusOK)
	body := rec.Body.String()
	for _, v := range []string{plainSecretValue, base64SecretValue, storedSecretValue, newVal, "extra-value-qqq", "another-stored-secret-value"} {
		if strings.Contains(body, v) {
			t.Fatalf("diff leaked a value %q: %s", v, body)
		}
	}
	var res envDiffResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if strings.Join(res.Added, ",") != "EXTRA" || strings.Join(res.Removed, ",") != "QUARTER_V1" || strings.Join(res.Changed, ",") != "API_SECRET,PORT" || res.SecretUnknown != 1 {
		t.Fatalf("unexpected diff: %+v", res)
	}
	f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/web/env-diff?against=preview", "", http.StatusOK)
	f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/web/env-diff", "", http.StatusBadRequest)
	f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/web/env-diff?against=nope", "", http.StatusNotFound)
	f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/web/env-diff?against=environment:env_missing", "", http.StatusNotFound)
	f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/missing/env-diff?against=web", "", http.StatusNotFound)
}

func TestOrgIsolation_EnvDiff(t *testing.T) {
	f := newOrgFixture(t)
	for _, c := range []struct {
		name, path string
		cookie     *http.Cookie
	}{
		{"org B reads org A app", "/api/v1/apps/" + appA + "/env-diff?against=preview", f.bOwner},
		{"org B diffs own app against org A", "/api/v1/apps/" + appB + "/env-diff?against=" + appA, f.bOwner},
		{"org A diffs against org B app", "/api/v1/apps/" + appA + "/env-diff?against=" + appB, f.cookies[store.TeamRoleOwner]},
	} {
		code, body := teamDo(t, f.rt, http.MethodGet, c.path, "", withCookie(c.cookie))
		if code != http.StatusNotFound {
			t.Errorf("%s: got %d %s, want 404", c.name, code, body)
		}
		for _, m := range orgBMarkers {
			if strings.Contains(body, m) && c.cookie != f.bOwner {
				t.Errorf("%s: body leaks %q", c.name, m)
			}
		}
	}
}
