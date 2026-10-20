package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAppScopedReadRoutes_IAMDenyScopedToApp(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	bootstrapTestAdmin(t, db)
	seedScheduledTaskApp(t, db, "secret-app")
	seedScheduledTaskApp(t, db, "open-app")
	abilities := []string{AbilityRead, AbilityRoot}
	denied := storeUserWithAbilitiesForTest(t, db, "denied@example.com", abilities)
	plain := storeUserWithAbilitiesForTest(t, db, "plain@example.com", abilities)
	deniedCookie := sessionCookieForTest(t, rt, denied.ID)
	plainCookie := sessionCookieForTest(t, rt, plain.ID)
	for _, ability := range abilities {
		attachTestPolicy(t, db, "deny-secret-"+ability, "Deny", ability, "app:secret-app", store.PrincipalTypeUser, denied.ID)
	}

	paths := []string{
		"secrets", "terminal", "exec-access", "badge", "deploys", "deploys/d1/logs", "logs", "logs/download",
		"metrics", "alerts", "tags", "domains/x.example.com/waf", "volumes/data/backups",
		"volumes/data/backup-schedule", "scheduled-tasks", "flags", "network", "git-source", "log-drain",
	}
	do := func(cookie *http.Cookie, app, path string) int {
		w := httptest.NewRecorder()
		rt.Handler().ServeHTTP(w, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/"+app+"/"+path, ""))
		return w.Code
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			if got := do(deniedCookie, "secret-app", path); got != http.StatusForbidden {
				t.Errorf("denied user on secret-app = %d, want 403", got)
			}
			if got := do(deniedCookie, "open-app", path); got == http.StatusForbidden || got == http.StatusUnauthorized {
				t.Errorf("denied user on open-app = %d, want the handler's normal status", got)
			}
			if got := do(plainCookie, "secret-app", path); got == http.StatusForbidden || got == http.StatusUnauthorized {
				t.Errorf("user without policies on secret-app = %d, want the handler's normal status", got)
			}
		})
	}
}

func TestNetworkTopology_HidesIAMDeniedApps(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	bootstrapTestAdmin(t, db)
	seedScheduledTaskApp(t, db, "secret-app")
	seedScheduledTaskApp(t, db, "open-app")
	denied := storeUserWithAbilitiesForTest(t, db, "denied@example.com", []string{AbilityRead})
	cookie := sessionCookieForTest(t, rt, denied.ID)
	attachTestPolicy(t, db, "deny-secret", "Deny", AbilityRead, "app:secret-app", store.PrincipalTypeUser, denied.ID)

	w := httptest.NewRecorder()
	rt.Handler().ServeHTTP(w, authedRequest(t, cookie, http.MethodGet, "/api/v1/network/topology", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "secret-app") {
		t.Errorf("topology leaked a denied app: %s", body)
	}
	if !strings.Contains(body, "open-app") {
		t.Errorf("topology lost a visible app: %s", body)
	}
}

func TestContainerHidden(t *testing.T) {
	prefixes := []string{"secret-app-"}
	if !containerHidden("secret-app-76064f79", prefixes) || containerHidden("open-app-1234", prefixes) || containerHidden("db-main", nil) {
		t.Error("containerHidden prefix matching is wrong")
	}
}
