package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

func envRevisionsDenyFeature() authzDenyFeature {
	return authzDenyFeature{
		name:  "env-revisions",
		match: func(_, p string) bool { return strings.Contains(p, "/env/revisions") },
		routes: []authzDenyRoute{
			// AbilityRoot here only widens who is tested for 403: the routes need read:sensitive, which read and developer callers lack.
			{http.MethodGet, "/api/v1/apps/{name}/env/revisions", "", AbilityRoot, false},
			{http.MethodGet, "/api/v1/apps/{name}/env/revisions/diff", "", AbilityRoot, false},
			{http.MethodPost, "/api/v1/apps/{name}/env/revisions/{rev}/restore", `{"confirm":"x1"}`, AbilityWrite, false},
		},
	}
}

func TestAuthzDeny_EnvRevisions(t *testing.T) {
	runAuthzDeny(t, envRevisionsDenyFeature())
}

func TestEnvRevisionRestore_NewRevisionNoValuesAndAudit(t *testing.T) {
	f := newEnvFixture(t)
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web", `{"name":"web","image":"img:v1","port":3000,"env":{"PORT":"3000","KEEP":"one","API_SECRET":"`+plainSecretValue+`"}}`, http.StatusOK)
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web", `{"name":"web","image":"img:v1","port":3000,"env":{"PORT":"3001","KEEP":"one","NEWVAR":"`+storedSecretValue+`"}}`, http.StatusOK)

	f.do(t, f.cookie, http.MethodPost, "/api/v1/apps/web/env/revisions/1/restore", `{"confirm":"nope"}`, http.StatusBadRequest)
	f.do(t, f.cookie, http.MethodPost, "/api/v1/apps/web/env/revisions/1/restore", `{}`, http.StatusBadRequest)
	f.do(t, f.cookie, http.MethodPost, "/api/v1/apps/web/env/revisions/99/restore", `{"confirm":"web"}`, http.StatusNotFound)
	f.do(t, f.cookie, http.MethodPost, "/api/v1/apps/ghost/env/revisions/1/restore", `{"confirm":"ghost"}`, http.StatusNotFound)

	rec := f.do(t, f.cookie, http.MethodPost, "/api/v1/apps/web/env/revisions/1/restore", `{"confirm":"web"}`, http.StatusOK)
	var got envRevisionRestoreResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Restored != 1 || got.Revision != 3 || strings.Join(got.Added, ",") != "API_SECRET" || strings.Join(got.Removed, ",") != "NEWVAR" || strings.Join(got.Changed, ",") != "PORT" {
		t.Fatalf("restore = %+v", got)
	}
	assertNoSecretValues(t, "restore response", rec.Body.String())

	svc, err := f.db.GetDesiredService(context.Background(), "web")
	if err != nil || svc.Env["PORT"] != "3000" || svc.Env["API_SECRET"] != plainSecretValue || !svc.EnvDirty {
		t.Fatalf("restored app = %+v, %v", svc, err)
	}
	// History stays: revisions 1 and 2 survive and the restore is revision 3.
	list := f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/web/env/revisions", "", http.StatusOK)
	var revs envRevisionsResponse
	if err := json.Unmarshal(list.Body.Bytes(), &revs); err != nil || len(revs.Revisions) != 3 || revs.Revisions[0].Rev != 3 {
		t.Fatalf("revisions = %+v, %v", revs, err)
	}
	entries, err := f.db.ListAuditEntries(context.Background(), 500, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var audited bool
	for _, e := range entries {
		raw, _ := json.Marshal(e)
		assertNoSecretValues(t, "audit row", string(raw))
		audited = audited || e.Action == "app.env.restore"
	}
	if !audited {
		t.Fatal("restore wrote no audit entry")
	}
}

func TestEnvRevisions_DiffUnreadableSnapshotIsExplicit(t *testing.T) {
	f := newEnvFixture(t)
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web", `{"name":"web","image":"img:v1","port":3000,"env":{"PORT":"3000"}}`, http.StatusOK)
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web", `{"name":"web","image":"img:v1","port":3000,"env":{"PORT":"3001"}}`, http.StatusOK)
	if _, err := f.db.ExecContext(context.Background(), `UPDATE env_revisions SET snapshot = ? WHERE rev = 1`, []byte("not-a-sealed-blob")); err != nil {
		t.Fatal(err)
	}
	f.do(t, f.cookie, http.MethodPost, "/api/v1/auth/step-up", stepUpBody(testAdminPassword, stepUpActionReveal), http.StatusOK)
	rec := f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/web/env/revisions/diff?from=1&to=2", "", http.StatusOK)
	var d envRevisionDiffResource
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil || !d.Unreadable || len(d.Changed) != 0 {
		t.Fatalf("diff = %+v, %v, want unreadable", d, err)
	}
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

	hidden := f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/web/env/revisions/diff?from=1&to=2", "", http.StatusOK)
	var h envRevisionDiffResource
	if err := json.Unmarshal(hidden.Body.Bytes(), &h); err != nil {
		t.Fatal(err)
	}
	if len(h.Changed) != 0 || !h.ChangedHidden {
		t.Fatalf("diff without step-up = %+v, want changed hidden", h)
	}
	f.do(t, f.cookie, http.MethodPost, "/api/v1/auth/step-up", stepUpBody(testAdminPassword, stepUpActionReveal), http.StatusOK)
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

func TestPutAppEnv_BatchSetUnsetNamesOnly(t *testing.T) {
	f := newEnvFixture(t)
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web", `{"name":"web","image":"img:v1","port":3000,"env":{"KEEP":"one","DROP":"two"}}`, http.StatusOK)
	rec := f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web/env", `{"set":{"NEW":"`+plainSecretValue+`","KEEP":"one"},"unset":["DROP","GONE"]}`, http.StatusOK)
	var got envMergeResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Changed || !got.EnvDirty || strings.Join(got.Set, ",") != "NEW" || strings.Join(got.Removed, ",") != "DROP" || strings.Join(got.Missing, ",") != "GONE" || strings.Join(got.Keys, ",") != "KEEP,NEW" {
		t.Fatalf("merge = %+v", got)
	}
	assertNoSecretValues(t, "merge response", rec.Body.String())
	svc, err := f.db.GetDesiredService(context.Background(), "web")
	if err != nil || svc.Env["NEW"] != plainSecretValue || svc.Image != "img:v1" || !svc.EnvDirty {
		t.Fatalf("stored = %+v, %v", svc, err)
	}
	// Repeating the batch is a no-op.
	again := f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web/env", `{"set":{"NEW":"`+plainSecretValue+`"},"unset":["DROP"]}`, http.StatusOK)
	var second envMergeResource
	if err := json.Unmarshal(again.Body.Bytes(), &second); err != nil || second.Changed || len(second.Set) != 0 {
		t.Fatalf("repeat = %+v, %v", second, err)
	}
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web/env", `{}`, http.StatusBadRequest)
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web/env", `{"set":{"A":"1"},"unset":["A"]}`, http.StatusBadRequest)
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/ghost/env", `{"set":{"A":"1"}}`, http.StatusNotFound)
}
