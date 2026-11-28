package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/spec"
	"github.com/glincker/levelrail/internal/store"
)

func TestCloneRefusesNamespaceNames(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"deleted-app/web", "git-source/x", "mail-tls-cert/x", "user-totp/x", "pr-env/..", "deleted-app%2Fweb", "a\\\\b"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/clone", `{"new_name":"`+name+`"}`))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("clone to %q: status %d body %s, want 400", name, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web%2Fx/clone", `{"new_name":"fine"}`))
	if rec.Code == http.StatusCreated {
		t.Fatalf("encoded slash in the path must not clone: %s", rec.Body.String())
	}
}

// Every service-name writer bottoms out in SaveDesiredService, so the store guard covers them all.
func TestServiceNameWritersRefuseNamespaceNames(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	ctx := context.Background()
	writers := map[string]func(name string) error{
		"store save": func(n string) error {
			return rt.apps.SaveDesiredService(ctx, store.DesiredService{Name: n, Image: "img:1", Port: 80})
		},
		"suspended save": func(n string) error {
			return db.SaveSuspendedDesiredService(ctx, store.DesiredService{Name: n, Image: "img:1", Port: 80})
		},
		"create validator": func(n string) error { return spec.ValidateServiceName(n) },
	}
	for label, write := range writers {
		for _, n := range []string{"deleted-app/web", "git-source/x", "a/b", "x\\y"} {
			if err := write(n); err == nil {
				t.Errorf("%s accepted %q", label, n)
			}
		}
	}
}

// A live app name passing the validator can never equal a parked namespace.
func TestLiveNameNeverEqualsParkedNamespace(t *testing.T) {
	t.Parallel()
	for _, other := range []string{"web", "api", "a-b"} {
		parked := "deleted-app/" + other
		if spec.ValidateServiceName(parked) == nil || !strings.Contains(parked, "/") {
			t.Fatalf("live name could equal %q", parked)
		}
	}
}
