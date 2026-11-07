package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

const projectCloneCanary = "CANARY-clone-5c19"

func TestProjectClone_ConfigOnlyNeverSecretsOrLiveDomains(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_c", Name: "shop", CreatedAt: "2026-08-16T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "acme/web:1", Port: 3000, Domains: []string{"shop.example.com"},
		Env:       map[string]string{"MODE": "prod", "API_TOKEN": projectCloneCanary},
		SecretEnv: []store.SecretEnvRef{{Name: "STRIPE_KEY"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateServiceProject(ctx, "web", "proj_c"); err != nil {
		t.Fatal(err)
	}

	clone := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects/proj_c/clone", body))
		return rec
	}

	dry := clone(`{"name":"shop","include":{"env_vars":true,"domains":"placeholder","settings":true},"dry_run":true}`)
	if dry.Code != http.StatusOK {
		t.Fatalf("dry run status = %d: %s", dry.Code, dry.Body.String())
	}
	if strings.Contains(dry.Body.String(), projectCloneCanary) {
		t.Fatal("dry run leaked a secret-looking value")
	}
	if list, _ := db.ListProjects(ctx); len(list) != 1 {
		t.Fatalf("dry run created a project: %d projects", len(list))
	}

	rec := clone(`{"name":"shop","include":{"env_vars":true,"domains":"none","settings":true}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("apply status = %d: %s", rec.Code, rec.Body.String())
	}
	var plan projectClonePlan
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.Applied || plan.Project.Name != "shop-2" || len(plan.Apps) != 1 || plan.Apps[0].Name != "web-copy" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	got, err := db.GetDesiredService(ctx, "web-copy")
	if err != nil || got == nil {
		t.Fatalf("clone app missing: %v", err)
	}
	if len(got.Domains) != 0 || got.Env["API_TOKEN"] != "" || got.Env["MODE"] != "prod" || !got.Suspended || got.ProjectID != plan.Project.ID {
		t.Fatalf("clone copied the wrong things: %+v", got)
	}
	keys := map[string]bool{}
	for _, k := range plan.SecretKeys {
		keys[k.Key] = true
	}
	if !keys["STRIPE_KEY"] || !keys["API_TOKEN"] {
		t.Fatalf("secret keys not listed: %+v", plan.SecretKeys)
	}
}
