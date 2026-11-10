package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func putSecretAs(rt *Router, tok, value string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/secrets/API_KEY", strings.NewReader(`{"value":"`+value+`"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestSecretProviderRefNeedsRoot(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	sensitive := seedMatrixToken(t, db, "ws1", []string{AbilityWriteSensitive})
	root := seedMatrixToken(t, db, "root1", []string{AbilityRoot})

	for _, v := range []string{"doppler://prod/OTHER_KEY", "infisical://inf/app/KEY"} {
		if rec := putSecretAs(rt, sensitive, v); rec.Code != http.StatusForbidden {
			t.Fatalf("write:sensitive storing %q = %d, want 403", v, rec.Code)
		}
	}
	if setter.calls != 0 {
		t.Fatalf("a denied provider reference reached the store (%d calls)", setter.calls)
	}
	if rec := putSecretAs(rt, sensitive, "plain-value"); rec.Code != http.StatusNoContent {
		t.Fatalf("plain value = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if rec := putSecretAs(rt, root, "doppler://prod/OTHER_KEY"); rec.Code != http.StatusNoContent {
		t.Fatalf("root provider reference = %d, want 204: %s", rec.Code, rec.Body.String())
	}
}
