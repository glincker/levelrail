package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/platformimport/dokployextras"
)

var extrasFixtureSecrets = []string{importFixtureValue, "AKIAFIXTUREKEY01", "fixture-secret-aws-value", "FIXTURE-SLACK-TOKEN", "fixture-smtp-password"}

func newExtrasSourceServer(t *testing.T) *httptest.Server {
	t.Helper()
	pages := map[string]string{
		"/api/project.all": `[{"projectId":"p1","name":"Shop","applications":[],"compose":[],"postgres":[{"postgresId":"pg1","name":"shop-db","dockerImage":"postgres:16",
			"backups":[{"backupId":"b1","schedule":"0 3 * * *","enabled":true,"destinationId":"d-aws","keepLatestCount":7}]}],"mysql":[],"mariadb":[],"mongo":[],"redis":[]}]`,
		"/api/destination.all": `[{"destinationId":"d-aws","name":"Nightly AWS","provider":"AWS","accessKey":"AKIAFIXTUREKEY01","secretAccessKey":"fixture-secret-aws-value","bucket":"shop-backups","region":"us-east-1","endpoint":""},
			{"destinationId":"d-dup","name":"taken-target","provider":"AWS","accessKey":"AKIAFIXTUREKEY01","secretAccessKey":"fixture-secret-aws-value","bucket":"other-bucket","region":"us-east-1","endpoint":""}]`,
		"/api/notification.all": `[{"notificationId":"n-mail","name":"On-call Mail","notificationType":"email","email":{"smtpServer":"smtp.example.com","password":"fixture-smtp-password","toAddresses":["oncall@example.com"]}},
			{"notificationId":"n-slack","name":"Ops Slack","notificationType":"slack","slack":{"webhookUrl":"https://hooks.invalid/services/FIXTURE-SLACK-TOKEN"}}]`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != importFixtureValue {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if body, ok := pages[r.URL.Path]; ok {
			_, _ = w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func extrasCall(t *testing.T, rt *Router, cookie *http.Cookie, path, body string) platformImportResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, path, body))
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status %d: %s", path, rec.Code, rec.Body.String())
	}
	for _, s := range extrasFixtureSecrets {
		if strings.Contains(rec.Body.String(), s) {
			t.Fatalf("%s leaked a secret value: %s", path, rec.Body.String())
		}
	}
	var resp platformImportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestPlatformImportExtrasPlanApplyUndo(t *testing.T) {
	t.Setenv(platformimport.AllowLoopbackEnv, "true")
	src := newExtrasSourceServer(t)
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithSecretSetter(&fakeSecretSetter{}), WithBackupSecrets(&fakeBackupSecretsSetter{}))
	cookie := loginTestSession(t, rt, db)
	base := `{"platform":"dokploy","url":"` + src.URL + `","token":"` + importFixtureValue + `","allow_loopback":true`

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets",
		`{"name":"taken-target","provider":"aws","bucket":"pre-existing","region":"us-east-1","access_key_id":"AKIAPREEXIST0001","secret_access_key":"pre-existing-value"}`))
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("seed target: %d %s", rec.Code, rec.Body.String())
	}

	plan := extrasCall(t, rt, cookie, "/api/v1/imports/platform/discover", base+`}`)
	if plan.Backups == nil || len(plan.Backups.Targets) != 2 || !plan.Backups.Targets[0].HasKeys || plan.Channels == nil || len(plan.Channels.Channels) != 2 {
		t.Fatalf("plan: %+v %+v", plan.Backups, plan.Channels)
	}
	if !strings.Contains(plan.Backups.Targets[0].Detail, "--access-key-id <id>") || plan.Channels.Channels[0].Enabled {
		t.Fatalf("plan without secrets must hint and default to disabled: %+v", plan.Channels.Channels[0])
	}

	applied := extrasCall(t, rt, cookie, "/api/v1/imports/platform/apply", base+`,"include_credentials":true}`)
	tg := applied.Backups.Targets
	if tg[0].State != dokployextras.StateCreated || tg[1].State != dokployextras.StateExists {
		t.Fatalf("targets: %+v", tg)
	}
	if s := applied.Backups.Schedules[0]; s.State != dokployextras.StateCreated || !s.Paused {
		t.Fatalf("schedule must be created paused: %+v", s)
	}
	if got, err := db.GetDesiredDatabase(t.Context(), "shop-db"); err != nil || !got.BackupPaused {
		t.Fatalf("database schedule not paused: %+v %v", got, err)
	}
	for _, c := range applied.Channels.Channels {
		if c.Enabled || (c.Kind == "slack" && c.State == dokployextras.StateCreated) {
			t.Fatalf("channel must stay disabled and the unresolvable slack URL refused: %+v", c)
		}
	}
	kinds := map[string]bool{}
	for _, r := range applied.Journal {
		if r.ID == "" || (r.Kind != dokployextras.KindBackupSchedule && r.Fingerprint == "") {
			t.Fatalf("journal entry without id or fingerprint: %+v", r)
		}
		kinds[r.Kind] = true
	}
	if !kinds[dokployextras.KindBackupTarget] || !kinds[dokployextras.KindBackupSchedule] {
		t.Fatalf("journal: %+v", applied.Journal)
	}

	forged := append([]dokployextras.Record(nil), applied.Journal...)
	for i := range forged {
		forged[i].Fingerprint, forged[i].Cron = "sha256:forged", "1 1 * * *"
	}
	undoBody, _ := json.Marshal(platformImportUndoRequest{Journal: forged})
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/imports/platform/undo", string(undoBody)))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"removed"`) {
		t.Fatalf("forged undo must remove nothing: %d %s", rec.Code, rec.Body.String())
	}

	undoBody, _ = json.Marshal(platformImportUndoRequest{Journal: applied.Journal})
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/imports/platform/undo", string(undoBody)))
	var undo platformImportUndoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &undo); err != nil || len(undo.Items) != len(applied.Journal) {
		t.Fatalf("undo: %v %s", err, rec.Body.String())
	}
	for _, it := range undo.Items {
		if it.Action != "removed" {
			t.Fatalf("undo item: %+v", it)
		}
	}
}

func TestPlatformImportExtrasOnlySkipsApps(t *testing.T) {
	t.Setenv(platformimport.AllowLoopbackEnv, "true")
	src := newExtrasSourceServer(t)
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithSecretSetter(&fakeSecretSetter{}), WithBackupSecrets(&fakeBackupSecretsSetter{}))
	cookie := loginTestSession(t, rt, db)
	body := `{"platform":"dokploy","url":"` + src.URL + `","token":"` + importFixtureValue + `","allow_loopback":true,"extras_only":true}`
	resp := extrasCall(t, rt, cookie, "/api/v1/imports/platform/apply", body)
	if _, err := db.GetDesiredDatabase(t.Context(), "shop-db"); err == nil {
		t.Fatal("extras_only must not import databases or apps")
	}
	if resp.Backups == nil || resp.Backups.Targets[0].State != dokployextras.StateNeedsSecret {
		t.Fatalf("without credentials the target must stay a hint: %+v", resp.Backups)
	}
	if resp.Backups.Schedules[0].State != dokployextras.StatePending {
		t.Fatalf("schedule without a target must be pending: %+v", resp.Backups.Schedules[0])
	}
}

func TestPlatformImportExtrasNeedOptInResumeAndSafeUndo(t *testing.T) {
	t.Setenv(platformimport.AllowLoopbackEnv, "true")
	src := newExtrasSourceServer(t)
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithSecretSetter(&fakeSecretSetter{}), WithBackupSecrets(&fakeBackupSecretsSetter{}))
	cookie := loginTestSession(t, rt, db)
	base := `{"platform":"dokploy","url":"` + src.URL + `","token":"` + importFixtureValue + `","allow_loopback":true`

	plain := extrasCall(t, rt, cookie, "/api/v1/imports/platform/apply", base+`}`)
	if len(plain.Journal) != 0 || plain.Backups.Targets[0].State == dokployextras.StateCreated {
		t.Fatalf("a plain apply must not create backups or channels: %+v", plain.Journal)
	}

	first := extrasCall(t, rt, cookie, "/api/v1/imports/platform/apply", base+`,"extras_only":true,"include_credentials":true}`)
	if len(first.Journal) == 0 {
		t.Fatal("first run created nothing")
	}
	prior, _ := json.Marshal(first.Journal)
	again := extrasCall(t, rt, cookie, "/api/v1/imports/platform/apply", base+`,"extras_only":true,"include_credentials":true,"journal":`+string(prior)+`}`)
	if len(again.Journal) != len(first.Journal) {
		t.Fatalf("resume must keep the first run's records: first %+v again %+v", first.Journal, again.Journal)
	}

	for _, bad := range []string{"../x", "a?b", "a#b"} {
		body, _ := json.Marshal(platformImportUndoRequest{Journal: []dokployextras.Record{{Kind: dokployextras.KindBackupTarget, Name: "t", ID: bad, Fingerprint: "sha256:x"}}})
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/imports/platform/undo", string(body)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("undo id %q: %d %s", bad, rec.Code, rec.Body.String())
		}
	}
}
