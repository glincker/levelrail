package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestHandleEnvironmentClone_CopyDomainsRewritten(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedCloneFixture(t, db)
	ctx := context.Background()

	if _, _, err := db.EditServiceEnvironmentDomains(ctx, "web", "env_uat", func([]string) ([]string, error) {
		return []string{"uat.example.com"}, nil
	}); err != nil {
		t.Fatal(err)
	}

	body := `{"new_environment_name":"eu","copy_domains":true,"domain_rewrite":{"find":"example.com","replace":"eu.example.com"}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/environments/env_src/clone", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
	}

	var cloneName string
	apps, err := db.ListDesiredServices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range apps {
		if a.Name != "web" && a.Name != "worker" && len(a.Domains) > 0 {
			cloneName = a.Name
		}
	}
	if cloneName == "" {
		t.Fatal("no cloned app with domains found")
	}
	cloned, _ := db.GetDesiredService(ctx, cloneName)
	if !slices.Equal(cloned.Domains, []string{"web-staging.eu.example.com"}) {
		t.Fatalf("cloned default domains = %v", cloned.Domains)
	}
	sets, err := db.ListServiceEnvironmentDomains(ctx, cloneName)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sets["env_uat"], []string{"uat.eu.example.com"}) {
		t.Fatalf("cloned uat set = %v", sets["env_uat"])
	}
}

func TestHandleEnvironmentClone_CopyDomainsNeedsRewrite(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedCloneFixture(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/environments/env_src/clone", `{"new_environment_name":"eu","copy_domains":true}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}
