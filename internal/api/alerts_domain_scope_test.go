package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func seedScopeApp(t *testing.T, db *store.DB, name string, domains ...string) {
	t.Helper()
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: name, Image: "img:v1", Port: 3000, Domains: domains}); err != nil {
		t.Fatalf("seed app %q: %v", name, err)
	}
}

func domainDropBody(resourceID string) string {
	return `{"name":"drop","kind":"domain_traffic_drop","resource_id":"` + resourceID + `","enabled":true}`
}

func TestAlertDomainScope(t *testing.T) {
	t.Parallel()
	rt, db, adb := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedScopeApp(t, db, "web", "shop.example.com", "parked.example.com")
	seedScopeApp(t, db, "other", "theirs.example.com")
	if err := db.ParkServiceDomain(context.Background(), "web", "parked.example.com", "test", ""); err != nil {
		t.Fatalf("park: %v", err)
	}

	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	denied := map[string]string{
		"other app host": "domain:theirs.example.com",
		"parked host":    "domain:parked.example.com",
		"unknown host":   "domain:nowhere.example.com",
		"empty host":     "domain:",
		"uppercase":      "domain:Shop.example.com",
		"malformed":      "domain:not a host",
		"other app id":   "service:other",
		"wildcard":       "domain:*",
	}
	for label, rid := range denied {
		if rec := do(http.MethodPost, "/api/v1/apps/web/alerts", domainDropBody(rid)); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400, body = %s", label, rec.Code, rec.Body.String())
		}
	}

	rec := do(http.MethodPost, "/api/v1/apps/web/alerts", domainDropBody("domain:shop.example.com"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("scoped create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created ruleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ResourceID != "domain:shop.example.com" {
		t.Fatalf("ResourceID = %q", created.ResourceID)
	}
	stored, err := adb.GetRule(context.Background(), created.ID)
	if err != nil || stored.ResourceID != "domain:shop.example.com" {
		t.Fatalf("stored = %+v, err = %v", stored, err)
	}

	var listed []ruleResource
	if err := json.Unmarshal(do(http.MethodGet, "/api/v1/apps/web/alerts", "").Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ResourceID != "domain:shop.example.com" {
		t.Errorf("listed = %+v, want the scoped rule", listed)
	}

	// The other app cannot see, change or delete the scoped rule.
	if rec := do(http.MethodPut, "/api/v1/apps/other/alerts/"+created.ID, domainDropBody("domain:theirs.example.com")); rec.Code != http.StatusNotFound {
		t.Errorf("cross-app update = %d, want 404", rec.Code)
	}
	if rec := do(http.MethodDelete, "/api/v1/apps/other/alerts/"+created.ID, ""); rec.Code != http.StatusNotFound {
		t.Errorf("cross-app delete = %d, want 404", rec.Code)
	}

	// Update may move the scope to another owned host, back to app-wide, but not to a foreign host.
	if rec := do(http.MethodPut, "/api/v1/apps/web/alerts/"+created.ID, domainDropBody("domain:theirs.example.com")); rec.Code != http.StatusBadRequest {
		t.Errorf("update to foreign host = %d, want 400", rec.Code)
	}
	if rec := do(http.MethodPut, "/api/v1/apps/web/alerts/"+created.ID, domainDropBody("")); rec.Code != http.StatusOK {
		t.Fatalf("update to app-wide = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got, _ := adb.GetRule(context.Background(), created.ID); got.ResourceID != "service:web" {
		t.Errorf("after update ResourceID = %q, want service:web", got.ResourceID)
	}
	if rec := do(http.MethodPut, "/api/v1/apps/web/alerts/"+created.ID, domainDropBody("domain:shop.example.com")); rec.Code != http.StatusOK {
		t.Fatalf("update to owned host = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got, _ := adb.GetRule(context.Background(), created.ID); got.ResourceID != "domain:shop.example.com" {
		t.Errorf("after update ResourceID = %q", got.ResourceID)
	}
	if rec := do(http.MethodDelete, "/api/v1/apps/web/alerts/"+created.ID, ""); rec.Code != http.StatusNoContent {
		t.Errorf("owner delete = %d, want 204", rec.Code)
	}

	// Other kinds ignore a body resource_id.
	rec = do(http.MethodPost, "/api/v1/apps/web/alerts", `{"name":"cpu","kind":"threshold","metric":"cpu_percent","comparator":">","threshold":80,"for_duration":"2m","resource_id":"domain:shop.example.com","enabled":true}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("threshold create = %d, body = %s", rec.Code, rec.Body.String())
	}
	var th ruleResource
	_ = json.Unmarshal(rec.Body.Bytes(), &th)
	if th.ResourceID != "service:web" {
		t.Errorf("threshold ResourceID = %q, want service:web", th.ResourceID)
	}
}
