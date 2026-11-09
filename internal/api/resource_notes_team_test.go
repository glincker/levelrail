package api

import (
	"context"
	"net/http"
	"regexp"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Another team's app or database answers exactly like a missing one, for cookies and scoped tokens alike.
func TestResourceNotesOtherTeamIs404(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	if err := f.db.SetResourceNotes(ctx, store.LabelKindApp, appB, store.ResourceNotes{Notes: "team b secret runbook"}); err != nil {
		t.Fatal(err)
	}
	plain := "notes-scoped-token-a"
	if err := f.db.SaveAPIToken(ctx, store.APIToken{ID: "tok-notes-a", Name: "tok-notes-a", TokenHash: hashToken(plain), Abilities: []string{AbilityRead, AbilityWrite}, OwnerUserID: f.members[store.TeamRoleOwner].ID}); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetAPITokenScope(ctx, store.APITokenScope{TokenID: "tok-notes-a", TeamID: teamA, Role: store.TeamRoleOwner}); err != nil {
		t.Fatal(err)
	}
	callers := map[string]func(*http.Request){
		"developer cookie": func(r *http.Request) { r.AddCookie(f.cookies[store.TeamRoleDeveloper]) },
		"owner cookie":     func(r *http.Request) { r.AddCookie(f.cookies[store.TeamRoleOwner]) },
		"scoped token":     bearer(plain),
	}
	for label, auth := range callers {
		for _, method := range []string{http.MethodGet, http.MethodPut} {
			_, missing := teamDo(t, f.rt, method, "/api/v1/apps/no-such-app/notes", `{"notes":"x"}`, auth)
			for _, p := range []string{"/api/v1/apps/" + appB + "/notes", "/api/v1/databases/" + dbB + "/notes"} {
				code, body := teamDo(t, f.rt, method, p, `{"notes":"overwritten"}`, auth)
				if code != http.StatusNotFound {
					t.Errorf("%s %s %s = %d %s, want 404", label, method, p, code, body)
				}
				if p == "/api/v1/apps/"+appB+"/notes" && stripRequestID(body) != stripRequestID(missing) {
					t.Errorf("%s %s: other team body %q differs from missing app body %q", label, method, body, missing)
				}
			}
		}
		if code, body := teamDo(t, f.rt, http.MethodPut, "/api/v1/apps/"+appA+"/notes", `{"description":"own team"}`, auth); code != http.StatusOK {
			t.Errorf("%s own team put = %d %s", label, code, body)
		}
	}
	if got, _ := f.db.GetResourceNotes(ctx, store.LabelKindApp, appB); got.Notes != "team b secret runbook" {
		t.Fatalf("team B notes changed: %+v", got)
	}
}

var notesRequestIDField = regexp.MustCompile(`"request_id":"[^"]*"`)

func stripRequestID(body string) string { return notesRequestIDField.ReplaceAllString(body, "") }
