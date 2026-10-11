package api

import (
	"context"
	"net/http"
	"testing"
)

// TestInstallImportedTemplate installs a converted template through the
// real install route: the address variables resolve to the chosen domain
// and the generated passwords are secret-backed and shared by both services.
func TestInstallImportedTemplate(t *testing.T) {
	rt, db := newTestRouterWithComposeSecrets(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/service-templates/gitea-with-postgresql/install",
		`{"name":"code","domains":{"gitea":"code.example.com"}}`, http.StatusOK)

	gitea, err := db.GetDesiredService(ctx, "code-gitea")
	if err != nil {
		t.Fatalf("GetDesiredService(gitea) error = %v", err)
	}
	if len(gitea.Domains) != 1 || gitea.Domains[0] != "code.example.com" {
		t.Errorf("gitea domains = %v", gitea.Domains)
	}
	if gitea.Env["GITEA__database__NAME"] != "gitea" {
		t.Errorf("database name = %q, want the template default", gitea.Env["GITEA__database__NAME"])
	}
	if _, ok := gitea.Env["GITEA__database__PASSWD"]; ok {
		t.Error("the generated database password landed in plain env")
	}
	pg, err := db.GetDesiredService(ctx, "code-postgresql")
	if err != nil {
		t.Fatalf("GetDesiredService(postgresql) error = %v", err)
	}
	if _, ok := pg.Env["POSTGRES_PASSWORD"]; ok {
		t.Error("POSTGRES_PASSWORD landed in plain env")
	}
	if len(gitea.SecretEnv) == 0 || len(pg.SecretEnv) == 0 {
		t.Errorf("secret env gitea %v postgres %v, want both secret-backed", gitea.SecretEnv, pg.SecretEnv)
	}
}

// TestInstallImportedTemplateWithoutDomain: an address variable with no
// domain chosen must be reported as needing configuration, not deployed
// with a literal token.
func TestInstallImportedTemplateWithoutDomain(t *testing.T) {
	rt, db := newTestRouterWithComposeSecrets(t)
	cookie := loginTestSession(t, rt, db)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/service-templates/n8n-with-postgresql/install",
		`{"name":"flows"}`, http.StatusBadRequest)
}
