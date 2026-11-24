package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func envRevisionsDenyFeature() authzDenyFeature {
	return authzDenyFeature{
		name:  "env-revisions",
		match: func(_, p string) bool { return strings.Contains(p, "/env/revisions") },
		routes: []authzDenyRoute{
			// AbilityRoot here only widens who is tested for 403: the routes need read:sensitive, which read and developer callers lack.
			{http.MethodGet, "/api/v1/apps/{name}/env/revisions", "", AbilityRoot, false},
			{http.MethodGet, "/api/v1/apps/{name}/env/revisions/diff", "", AbilityRoot, false},
		},
	}
}

func TestAuthzDeny_EnvRevisions(t *testing.T) {
	runAuthzDeny(t, envRevisionsDenyFeature())
}

func TestEnvRevisions_SnapshotAndNamesOnlyDiff(t *testing.T) {
	f := newEnvFixture(t)
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web", `{"name":"web","image":"img:v1","port":3000,"env":{"PORT":"3000","KEEP":"one","API_SECRET":"`+plainSecretValue+`"}}`, http.StatusOK)
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web", `{"name":"web","image":"img:v1","port":3000,"env":{"PORT":"3001","KEEP":"one","NEWVAR":"`+storedSecretValue+`"}}`, http.StatusOK)

	list := f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/web/env/revisions", "", http.StatusOK)
	var revs envRevisionsResponse
	if err := json.Unmarshal(list.Body.Bytes(), &revs); err != nil {
		t.Fatal(err)
	}
	if len(revs.Revisions) != 2 || revs.Revisions[0].Rev != 2 {
		t.Fatalf("revisions = %+v, want 2 newest first", revs.Revisions)
	}
	assertNoSecretValues(t, "revision list", list.Body.String())

	rec := f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/web/env/revisions/diff?from=1&to=2", "", http.StatusOK)
	var d envRevisionDiffResource
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if strings.Join(d.Added, ",") != "NEWVAR" || strings.Join(d.Removed, ",") != "API_SECRET" || strings.Join(d.Changed, ",") != "PORT" {
		t.Fatalf("diff = %+v", d)
	}
	assertNoSecretValues(t, "revision diff", rec.Body.String())
	if strings.Contains(rec.Body.String(), "3001") || strings.Contains(rec.Body.String(), "one") {
		t.Fatalf("diff leaks a value: %s", rec.Body.String())
	}
	f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/web/env/revisions/diff?from=1&to=99", "", http.StatusNotFound)
	f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/web/env/revisions/diff?from=x", "", http.StatusBadRequest)
}
