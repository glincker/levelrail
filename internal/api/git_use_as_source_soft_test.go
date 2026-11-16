package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/gitlabapp"
)

func TestGitUseAsSource_GitLabWebhookFailureIsSoft(t *testing.T) {
	fake := &fakeGitLabAppClient{
		project:       gitlabapp.Project{ID: 7, Name: "web", PathWithNamespace: "bob/web", HTTPURLToRepo: "https://gitlab.com/bob/web.git", DefaultBranch: "trunk"},
		createHookErr: http.ErrHandlerTimeout,
	}
	secrets := newFakeGitConnSecrets()
	rt, db := newTestRouterWithGitLabAndGitSource(t, secrets, fake, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	setPrimaryDomain(t, db)
	b := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.com", "bob", true)
	seedApp(t, db, "web")

	rec := gcRequest(t, rt, cookie, http.MethodPost, "/api/v1/git-connections/"+b.ID+"/use-as-source", `{"app_name":"web","repo":"7"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d body %s, want 201 with the source connected", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"webhook_registered":false`) || !strings.Contains(rec.Body.String(), "webhook_error") {
		t.Errorf("body %s lacks webhook_registered:false and webhook_error", rec.Body.String())
	}
	if _, err := db.GetGitSource(t.Context(), "web"); err != nil {
		t.Errorf("git source not stored: %v", err)
	}
}
