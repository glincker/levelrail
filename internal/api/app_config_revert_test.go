package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func saveEnv(t *testing.T, f *teamFixture, app string, mutate func(*store.DesiredService)) {
	t.Helper()
	ctx := context.Background()
	svc, err := f.db.GetDesiredService(ctx, app)
	if err != nil {
		t.Fatal(err)
	}
	mutate(svc)
	if err := f.db.SaveDesiredService(ctx, *svc); err != nil {
		t.Fatal(err)
	}
	f.rt.recordConfigRevision(ctx, app, "tester", 0)
}

func envOf(t *testing.T, f *teamFixture, app string) map[string]string {
	t.Helper()
	svc, err := f.db.GetDesiredService(context.Background(), app)
	if err != nil {
		t.Fatal(err)
	}
	return svc.Env
}

func TestRevertApp_RecordThenRevert(t *testing.T) {
	f := newTeamFixture(t)
	saveEnv(t, f, appA, func(s *store.DesiredService) { s.Env = map[string]string{"MODE": "prod"} })
	saveEnv(t, f, appA, func(s *store.DesiredService) {
		s.Env = map[string]string{"MODE": "broken", "API_TOKEN": "s3cr3t-value-123"}
	})
	owner := withCookie(f.cookies[store.TeamRoleOwner])

	code, body := teamDo(t, f.rt, http.MethodGet, "/api/v1/apps/"+appA+"/changes", "", owner)
	if code != http.StatusOK || strings.Contains(body, "s3cr3t-value-123") {
		t.Fatalf("changes = %d, secret in body: %v", code, strings.Contains(body, "s3cr3t"))
	}
	// Revision 1 is the first recorded state, MODE=prod.
	code, body = teamDo(t, f.rt, http.MethodPost, "/api/v1/apps/"+appA+"/revert", `{"to":1,"dry_run":true}`, owner)
	if code != http.StatusOK || strings.Contains(body, "s3cr3t-value-123") {
		t.Fatalf("dry run = %d %s", code, body)
	}
	if env := envOf(t, f, appA); env["MODE"] != "broken" {
		t.Fatalf("dry run changed env: %v", env)
	}
	revs, _ := f.db.ListAppConfigRevisions(context.Background(), appA, 0)
	before := len(revs)

	code, body = teamDo(t, f.rt, http.MethodPost, "/api/v1/apps/"+appA+"/revert", `{"to":1}`, owner)
	if code != http.StatusOK || strings.Contains(body, "s3cr3t-value-123") {
		t.Fatalf("revert = %d %s", code, body)
	}
	env := envOf(t, f, appA)
	if env["MODE"] != "prod" {
		t.Fatalf("env not restored: %v", env)
	}
	revs, _ = f.db.ListAppConfigRevisions(context.Background(), appA, 0)
	if len(revs) != before+1 || revs[0].RevertOf != 1 {
		t.Fatalf("revert not recorded as a new revision: %d revisions, newest revert_of %d", len(revs), revs[0].RevertOf)
	}
	for _, r := range revs {
		if strings.Contains(r.Snapshot+r.Changes, "s3cr3t-value-123") {
			t.Fatal("history holds a secret value")
		}
	}
}

func TestRevertApp_DeniedAndIsolated(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	saveEnv(t, f, appA, func(s *store.DesiredService) { s.Env = map[string]string{"MODE": "one"} })
	saveEnv(t, f, appA, func(s *store.DesiredService) { s.Env = map[string]string{"MODE": "two"} })
	saveEnv(t, f, appB, func(s *store.DesiredService) { s.Env = map[string]string{"MODE": "b-one"} })
	saveEnv(t, f, appB, func(s *store.DesiredService) { s.Env = map[string]string{"MODE": "b-two"} })

	viewer := withCookie(f.cookies[store.TeamRoleViewer])
	if code, _ := teamDo(t, f.rt, http.MethodPost, "/api/v1/apps/"+appA+"/revert", `{"to":1}`, viewer); code != http.StatusForbidden {
		t.Fatalf("read-only caller got %d", code)
	}
	if envOf(t, f, appA)["MODE"] != "two" {
		t.Fatal("read-only caller changed env")
	}

	owner := withCookie(f.cookies[store.TeamRoleOwner])
	code, _ := teamDo(t, f.rt, http.MethodPost, "/api/v1/apps/"+appB+"/revert", `{"to":1}`, owner)
	if code != http.StatusNotFound && code != http.StatusForbidden {
		t.Fatalf("other team caller got %d", code)
	}
	missing, _ := teamDo(t, f.rt, http.MethodPost, "/api/v1/apps/zz-missing-app/revert", `{"to":1}`, owner)
	if code != missing {
		t.Fatalf("other team answers %d, a missing app %d", code, missing)
	}
	svc, _ := f.db.GetDesiredService(ctx, appB)
	if svc.Env["MODE"] != "b-two" {
		t.Fatal("other team caller changed env")
	}
}

func TestRevertApp_TargetMissing(t *testing.T) {
	f := newTeamFixture(t)
	owner := withCookie(f.cookies[store.TeamRoleOwner])
	saveEnv(t, f, appA, func(s *store.DesiredService) { s.SecretEnv = []store.SecretEnvRef{{Name: "DB_PASS"}} })
	saveEnv(t, f, appA, func(s *store.DesiredService) { s.SecretEnv = nil })
	code, body := teamDo(t, f.rt, http.MethodPost, "/api/v1/apps/"+appA+"/revert", `{"to":1}`, owner)
	if code != http.StatusConflict || !strings.Contains(body, "revert_target_missing") {
		t.Fatalf("missing secret = %d %s", code, body)
	}

	saveEnv(t, f, appA, func(s *store.DesiredService) { s.Domains = []string{"gone.example.test"} })
	saveEnv(t, f, appA, func(s *store.DesiredService) { s.Domains = nil })
	saveEnv(t, f, appA2, func(s *store.DesiredService) { s.Domains = []string{"gone.example.test"} })
	revs, _ := f.db.ListAppConfigRevisions(context.Background(), appA, 0)
	var to int
	for _, r := range revs {
		if strings.Contains(r.Snapshot, "gone.example.test") {
			to = r.N
		}
	}
	code, body = teamDo(t, f.rt, http.MethodPost, "/api/v1/apps/"+appA+"/revert", `{"to":`+strconv.Itoa(to)+`}`, owner)
	if code != http.StatusConflict || !strings.Contains(body, "revert_target_missing") {
		t.Fatalf("taken domain = %d %s", code, body)
	}
	if code, _ := teamDo(t, f.rt, http.MethodPost, "/api/v1/apps/"+appA+"/revert", `{"to":999}`, owner); code != http.StatusNotFound {
		t.Fatalf("unknown revision = %d", code)
	}
}
