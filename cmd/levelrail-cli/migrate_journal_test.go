package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// journalTarget is a fake control plane holding apps in memory, refusing creates named in fail.
type journalTarget struct {
	mu      sync.Mutex
	apps    map[string]appResource
	fail    map[string]bool
	deploys map[string]int
	calls   []string
	srv     *httptest.Server
}

func newJournalTarget(t *testing.T, existing ...string) *journalTarget {
	t.Helper()
	jt := &journalTarget{apps: map[string]appResource{}, fail: map[string]bool{}, deploys: map[string]int{}}
	for _, n := range existing {
		jt.apps[n] = appResource{Name: n, Image: "pre:1"}
	}
	jt.srv = httptest.NewServer(http.HandlerFunc(jt.serve))
	t.Cleanup(jt.srv.Close)
	return jt
}

func (jt *journalTarget) serve(w http.ResponseWriter, r *http.Request) {
	jt.mu.Lock()
	defer jt.mu.Unlock()
	jt.calls = append(jt.calls, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
	w.Header().Set("Content-Type", "application/json")
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/apps/")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps":
		var body appResource
		_ = json.NewDecoder(r.Body).Decode(&body)
		if jt.fail[body.Name] {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
			return
		}
		if _, ok := jt.apps[body.Name]; ok {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"exists"}`))
			return
		}
		jt.apps[body.Name] = body
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(body)
	case r.URL.Path == "/api/v1/deployments":
		items := make([]map[string]string, jt.deploys[r.URL.Query().Get("app")])
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
	case strings.Contains(name, "/"):
		_, _ = w.Write([]byte(`{}`))
	case r.Method == http.MethodGet:
		app, ok := jt.apps[name]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(app)
	case r.Method == http.MethodDelete:
		if _, ok := jt.apps[name]; !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		delete(jt.apps, name)
		_, _ = w.Write([]byte(`{"status":"deleted"}`))
	default:
		_, _ = w.Write([]byte(`{}`))
	}
}

func (jt *journalTarget) has(name string) bool {
	jt.mu.Lock()
	defer jt.mu.Unlock()
	_, ok := jt.apps[name]
	return ok
}

func (jt *journalTarget) count(prefix string) int {
	jt.mu.Lock()
	defer jt.mu.Unlock()
	n := 0
	for _, c := range jt.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func journalApps(names ...string) *migrationReport {
	r := &migrationReport{CoolifyURL: "https://source.example.test"}
	for _, n := range names {
		r.Apps = append(r.Apps, mappedApp{SourceName: n, ServiceName: n, Service: mappedService{Port: 8080}})
	}
	return r
}

func runJournaledApply(t *testing.T, jt *journalTarget, path string, resume bool, report *migrationReport) *applyJournal {
	t.Helper()
	jr, err := openApplyJournal("cli-test", "coolify", path, report.CoolifyURL, jt.srv.URL, resume, envMap())
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	defer jr.close()
	applyMigrationJournaled(context.Background(), NewClient(jt.srv.URL, "fake-token"), report, jr)
	return jr
}

func runUndo(t *testing.T, jt *journalTarget, path, stdin string, extra ...string) (int, string) {
	t.Helper()
	var out, errb bytes.Buffer
	args := append([]string{"--journal", path, "--target-api-url", jt.srv.URL, "--target-token", "fake-token"}, extra...)
	code := runMigrateUndo("cli-test", "coolify", args, strings.NewReader(stdin), &out, &errb, envMap())
	return code, out.String() + errb.String()
}

func TestMigrateJournal_PartialFailureThenUndoRemovesOnlyCreated(t *testing.T) {
	jt := newJournalTarget(t, "legacy", "keep")
	jt.fail["third"] = true
	path := filepath.Join(t.TempDir(), "run.json")
	report := journalApps("first", "second", "third", "legacy")
	runJournaledApply(t, jt, path, false, report)
	if !report.Apps[0].Applied || !report.Apps[1].Applied || report.Apps[2].Applied || report.Apps[3].Applied {
		t.Fatalf("applied flags wrong: %+v", report.Apps)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("journal mode = %v, %v; want 0600", fi, err)
	}

	code, out := runUndo(t, jt, path, "")
	if code != exitOK || !strings.Contains(out, "dry run") || !jt.has("first") {
		t.Fatalf("default undo must be a dry run: code=%d out=%s", code, out)
	}
	code, out = runUndo(t, jt, path, "", "--yes")
	if code != exitOK {
		t.Fatalf("undo --yes: code=%d out=%s", code, out)
	}
	if jt.has("first") || jt.has("second") {
		t.Errorf("created apps still present: %s", out)
	}
	if !jt.has("legacy") || !jt.has("keep") {
		t.Errorf("pre-existing apps were removed: %s", out)
	}
	if n := jt.count("DELETE "); n != 2 {
		t.Errorf("delete calls = %d, want 2", n)
	}
	if !strings.Contains(out, "second") || strings.Index(out, "remove app second") > strings.Index(out, "remove app first") {
		t.Errorf("want reverse order in plan: %s", out)
	}
}

func TestMigrateJournal_ResumeAfterCrash(t *testing.T) {
	jt := newJournalTarget(t, "first", "outsider")
	path := filepath.Join(t.TempDir(), "run.json")
	now := time.Now().UTC().Format(time.RFC3339)
	j := &migrateJournal{Version: journalVersion, Source: "coolify", SourceHost: "https://source.example.test", TargetHost: originOf(jt.srv.URL),
		StartedAt: now, UpdatedAt: now, Resources: []journalResource{
			{Kind: journalKindApp, Name: "first", CreatedByRun: true, Fingerprint: appFingerprint(appResource{Name: "first", Image: "pre:1"}), Steps: []journalStep{{Step: stepCreated, State: stateDone, At: now}, {Step: stepGitSource, State: stateFailed, At: now}}},
			{Kind: journalKindApp, Name: "outsider", Steps: []journalStep{{Step: stepCreated, State: statePending, At: now}}},
		}}
	if err := saveJournal(path, j, time.Now()); err != nil {
		t.Fatal(err)
	}
	report := journalApps("first", "outsider", "second")
	report.Apps[0].RepoURL = "https://git.example.test/acme/first.git"
	jr := runJournaledApply(t, jt, path, true, report)

	if n := jt.count("POST /api/v1/apps?"); n != 1 {
		t.Errorf("creates = %d, want only the missing app", n)
	}
	if n := jt.count("PUT /api/v1/apps/first/git-source"); n != 1 {
		t.Errorf("git source retries = %d, want 1", n)
	}
	if report.Apps[1].Applied || !strings.Contains(report.Apps[1].CreateError, "cannot confirm") {
		t.Errorf("ambiguous pre-existing app must be left untouched: %+v", report.Apps[1])
	}
	if r := jr.j.find(journalKindApp, "outsider"); r == nil || r.CreatedByRun || r.state(stepCreated) != stateUnknown {
		t.Errorf("outsider recorded as %+v", r)
	}
	if r := jr.j.find(journalKindApp, "first"); r.state(stepGitSource) != stateDone {
		t.Errorf("first git source = %q, want done", r.state(stepGitSource))
	}

	jr = runJournaledApply(t, jt, path, true, journalApps("first", "outsider", "second"))
	if n := jt.count("POST /api/v1/apps?"); n != 1 {
		t.Errorf("second resume re-created something: %d creates", n)
	}
	if r := jr.j.find(journalKindApp, "outsider"); r.state(stepCreated) != stateUnknown {
		t.Errorf("unknown state lost on a second resume: %q", r.state(stepCreated))
	}
}

func TestMigrateJournal_ResumeWithoutFingerprintLeavesAppAlone(t *testing.T) {
	jt := newJournalTarget(t, "first")
	path := filepath.Join(t.TempDir(), "run.json")
	now := time.Now().UTC().Format(time.RFC3339)
	j := &migrateJournal{Version: journalVersion, Source: "coolify", SourceHost: "https://source.example.test", TargetHost: originOf(jt.srv.URL),
		StartedAt: now, UpdatedAt: now, Resources: []journalResource{{Kind: journalKindApp, Name: "first", CreatedByRun: true,
			Steps: []journalStep{{Step: stepCreated, State: stateDone, At: now}, {Step: stepSecrets, State: stateFailed, At: now}}}}}
	if err := saveJournal(path, j, time.Now()); err != nil {
		t.Fatal(err)
	}
	report := journalApps("first")
	report.Apps[0].Service.EnvLiteral = map[string]string{"API_KEY": "fake-api-key-value"}
	runJournaledApply(t, jt, path, true, report)
	if report.Apps[0].Applied || jt.count("PUT /api/v1/apps/first/secrets") != 0 {
		t.Errorf("resume wrote into an app it cannot identify: %+v", report.Apps[0])
	}
}

func TestMigrateUndo_FingerprintNeedsForceApp(t *testing.T) {
	jt := newJournalTarget(t)
	path := filepath.Join(t.TempDir(), "run.json")
	runJournaledApply(t, jt, path, false, journalApps("alpha", "beta"))
	j, err := loadJournal(path, "coolify", journalMaxBytesDflt)
	if err != nil {
		t.Fatal(err)
	}
	if j.find(journalKindApp, "alpha").Fingerprint == "" {
		t.Fatal("fingerprint not written after create")
	}
	for i := range j.Resources {
		j.Resources[i].Fingerprint = ""
	}
	if err := saveJournal(path, j, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, out := runUndo(t, jt, path, "", "--yes", "--force"); !jt.has("alpha") || !jt.has("beta") {
		t.Fatalf("--force removed an app with no fingerprint: %s", out)
	}
	if code, _ := runUndo(t, jt, path, "", "--yes", "--force-app", "gamma"); code == exitOK {
		t.Error("--force-app accepted a name the run did not create")
	}
	if _, out := runUndo(t, jt, path, "", "--yes", "--force-app", "alpha"); jt.has("alpha") || !jt.has("beta") {
		t.Errorf("--force-app alpha must remove only alpha: %s", out)
	}
}

func TestMigrateJournal_FingerprintRightAfterCreate(t *testing.T) {
	jt := newJournalTarget(t)
	path := filepath.Join(t.TempDir(), "run.json")
	jr, err := openApplyJournal("cli-test", "coolify", path, "https://source.example.test", jt.srv.URL, false, envMap())
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(jt.srv.URL, "fake-token")
	a := &journalApps("crashy").Apps[0]
	rec, _ := jr.beginApp(context.Background(), client, a)
	if _, err := client.CreateApp(context.Background(), migrationAppBody(a)); err != nil {
		t.Fatal(err)
	}
	jr.markCreated(context.Background(), client, rec)
	jr.close()
	// The run "crashes" here: no later step or seal ran.
	if code, out := runUndo(t, jt, path, "", "--yes"); code != exitOK || jt.has("crashy") {
		t.Errorf("crash recovery needed force: %d %s", code, out)
	}
}

func TestMigrateJournal_StaleLockAndSymlinkedFolder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.json")
	if err := os.WriteFile(path+journalLockSuffix, []byte("424242\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := lockJournal(path); err == nil || !strings.Contains(err.Error(), "pid 424242") {
		t.Errorf("stale lock message lacks the owner: %v", err)
	}
	realDir := filepath.Join(dir, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	jt := newJournalTarget(t)
	if _, err := openApplyJournal("cli-test", "coolify", filepath.Join(link, "run.json"), "https://s.example.test", jt.srv.URL, false, envMap()); err == nil {
		t.Error("journal in a symlinked folder accepted for writing")
	}
	runJournaledApply(t, jt, filepath.Join(realDir, "run.json"), false, journalApps("one"))
	if _, err := loadJournal(filepath.Join(link, "run.json"), "coolify", journalMaxBytesDflt); err == nil {
		t.Error("journal in a symlinked folder accepted for reading")
	}
}

func TestMigrateUndo_ModifiedAndLaterDeploys(t *testing.T) {
	jt := newJournalTarget(t)
	path := filepath.Join(t.TempDir(), "run.json")
	runJournaledApply(t, jt, path, false, journalApps("edited", "deployed", "clean"))
	jt.mu.Lock()
	a := jt.apps["edited"]
	a.Image = "changed:2"
	jt.apps["edited"] = a
	jt.deploys["deployed"] = 1
	jt.mu.Unlock()

	_, out := runUndo(t, jt, path, "", "--yes")
	if !jt.has("edited") || !jt.has("deployed") || jt.has("clean") {
		t.Fatalf("want only clean removed: %s", out)
	}
	if !strings.Contains(out, "changed since the run") || !strings.Contains(out, "1 deploy(s) since the run") {
		t.Errorf("plan must list why: %s", out)
	}
	_, out = runUndo(t, jt, path, "", "--yes", "--force")
	if !jt.has("edited") || jt.has("deployed") {
		t.Errorf("--force must remove only the deployed app, never a changed one: %s", out)
	}
	_, out = runUndo(t, jt, path, "", "--yes", "--force-app", "edited")
	if jt.has("edited") {
		t.Errorf("--force-app must remove the named app: %s", out)
	}
}

func TestMigrateUndo_IncludeDataNeedsTypedCount(t *testing.T) {
	jt := newJournalTarget(t)
	path := filepath.Join(t.TempDir(), "run.json")
	report := journalApps("store", "web")
	runJournaledApply(t, jt, path, false, report)
	jt.mu.Lock()
	a := jt.apps["store"]
	a.Volumes = []appVolumeResource{{Name: "store-data", ContainerPath: "/data"}}
	jt.apps["store"] = a
	jt.mu.Unlock()
	// Volumes added after the run change the fingerprint, so mark them recorded as at apply time.
	j, err := loadJournal(path, "coolify", journalMaxBytesDflt)
	if err != nil {
		t.Fatal(err)
	}
	j.find(journalKindApp, "store").Fingerprint = appFingerprint(a)
	if err := saveJournal(path, j, time.Now()); err != nil {
		t.Fatal(err)
	}

	_, out := runUndo(t, jt, path, "", "--yes")
	if !jt.has("store") || jt.has("web") || !strings.Contains(out, "--include-data") {
		t.Fatalf("data app must be skipped without --include-data: %s", out)
	}
	code, _ := runUndo(t, jt, path, "2\n", "--yes", "--include-data")
	if code == exitOK || !jt.has("store") {
		t.Fatalf("wrong typed count must abort: code=%d", code)
	}
	code, out = runUndo(t, jt, path, "1\n", "--yes", "--include-data")
	if code != exitOK || jt.has("store") || jt.count("DELETE /api/v1/apps/store?confirm=store&delete_data=true") != 1 {
		t.Fatalf("include-data delete: code=%d out=%s", code, out)
	}
}

func TestMigrateUndo_EditedJournalCannotReachOtherApps(t *testing.T) {
	jt := newJournalTarget(t, "prod")
	path := filepath.Join(t.TempDir(), "run.json")
	runJournaledApply(t, jt, path, false, journalApps("mine"))
	j, err := loadJournal(path, "coolify", journalMaxBytesDflt)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	forged := journalResource{Kind: journalKindApp, Name: "prod", CreatedByRun: true, Fingerprint: "sha256:" + strings.Repeat("0", 64),
		Steps: []journalStep{{Step: stepCreated, State: stateDone, At: now}}}
	j.Resources = append(j.Resources, forged, journalResource{Kind: journalKindApp, Name: "ghost", CreatedByRun: true, Steps: forged.Steps})
	if err := saveJournal(path, j, time.Now()); err != nil {
		t.Fatal(err)
	}
	_, out := runUndo(t, jt, path, "", "--yes", "--force")
	if !jt.has("prod") || jt.has("mine") {
		t.Fatalf("forged entry reached prod or real entry kept: %s", out)
	}
	if !strings.Contains(out, "ghost: already gone") {
		t.Errorf("missing app must be reported, not deleted: %s", out)
	}

	other := newJournalTarget(t, "mine")
	if code, out := runUndo(t, other, path, "", "--yes"); code == exitOK || !other.has("mine") {
		t.Errorf("undo against another control plane must be refused: %d %s", code, out)
	}
}

func TestMigrateJournal_HostileFilesRefused(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	valid := func(mut func(*migrateJournal)) []byte {
		j := migrateJournal{Version: journalVersion, Source: "coolify", TargetHost: "https://cp.example.test", StartedAt: now, UpdatedAt: now,
			Resources: []journalResource{{Kind: journalKindApp, Name: "web", CreatedByRun: true, Steps: []journalStep{{Step: stepCreated, State: stateDone, At: now}}}}}
		mut(&j)
		b, _ := json.Marshal(j)
		return b
	}
	cases := map[string][]byte{
		"traversal name": valid(func(j *migrateJournal) { j.Resources[0].Name = "../etc/passwd" }),
		"slash name":     valid(func(j *migrateJournal) { j.Resources[0].Name = "a/b" }),
		"database kind":  valid(func(j *migrateJournal) { j.Resources[0].Kind = "database" }),
		"duplicate":      valid(func(j *migrateJournal) { j.Resources = append(j.Resources, j.Resources[0]) }),
		"bad step":       valid(func(j *migrateJournal) { j.Resources[0].Steps[0].Step = "rm -rf" }),
		"created no":     valid(func(j *migrateJournal) { j.Resources[0].Steps[0].State = stateFailed }),
		"bad host":       valid(func(j *migrateJournal) { j.TargetHost = "https://u:p@cp.example.test/x" }),
		"other source":   valid(func(j *migrateJournal) { j.Source = "dokploy" }),
		"unknown field":  []byte(`{"version":1,"source":"coolify","extra":true}`),
		"huge":           append(valid(func(*migrateJournal) {}), bytes.Repeat([]byte(" "), 4096)...),
	}
	dir := t.TempDir()
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(dir, strings.ReplaceAll(name, " ", "-")+".json")
			if err := os.WriteFile(p, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadJournal(p, "coolify", 2048); err == nil {
				t.Errorf("hostile journal %q accepted", name)
			}
		})
	}
	good := filepath.Join(dir, "good.json")
	if err := os.WriteFile(good, valid(func(*migrateJournal) {}), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadJournal(good, "coolify", 2048); err != nil {
		t.Fatalf("valid journal refused: %v", err)
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(good, link); err == nil {
		if _, err := loadJournal(link, "coolify", 2048); err == nil {
			t.Error("symlinked journal accepted")
		}
		if err := saveJournal(link, &migrateJournal{}, time.Now()); err == nil {
			t.Error("save through a symlink accepted")
		}
	}
	if err := os.Chmod(good, 0o644); err == nil && runtime.GOOS != "windows" { //nolint:gosec // the test needs a too-open mode to prove it is refused
		if _, err := loadJournal(good, "coolify", 2048); err == nil {
			t.Error("group readable journal accepted")
		}
	}
}

func TestMigrateJournal_LockAndNoOverwrite(t *testing.T) {
	jt := newJournalTarget(t)
	path := filepath.Join(t.TempDir(), "run.json")
	unlock, err := lockJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openApplyJournal("cli-test", "coolify", path, "https://s.example.test", jt.srv.URL, false, envMap()); err == nil || !strings.Contains(err.Error(), "in use") {
		t.Errorf("concurrent run not refused: %v", err)
	}
	unlock()
	runJournaledApply(t, jt, path, false, journalApps("one"))
	if _, err := openApplyJournal("cli-test", "coolify", path, "https://s.example.test", jt.srv.URL, false, envMap()); err == nil {
		t.Error("a fresh run overwrote an existing journal")
	}
}

func TestMigrateJournal_NeverHoldsSecrets(t *testing.T) {
	jt := newJournalTarget(t)
	path := filepath.Join(t.TempDir(), "run.json")
	report := journalApps("api")
	report.CoolifyURL = "https://user:fake-url-pass@source.example.test/path?token=fake-query-token"
	report.Apps[0].RepoURL = "https://fake-user:fake-repo-token@git.example.test/acme/api.git"
	report.Apps[0].Service.EnvLiteral = map[string]string{"DATABASE_URL": "fake-db-pass", "API_KEY": "fake-api-key-value"}
	report.Apps[0].Files = []mappedFile{{Path: "/app/.env", Content: []byte("FAKE_FILE_SECRET=fake-file-value"), Secret: true}}
	runJournaledApply(t, jt, path, false, report)
	data, err := os.ReadFile(path) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"fake-url-pass", "fake-query-token", "fake-repo-token", "fake-db-pass", "fake-api-key-value", "fake-file-value", "DATABASE_URL", "API_KEY", "fake-token"} {
		if strings.Contains(string(data), s) {
			t.Errorf("journal holds %q: %s", s, data)
		}
	}
	var j migrateJournal
	if err := json.Unmarshal(data, &j); err != nil || !slices.ContainsFunc(j.Resources, func(r journalResource) bool { return r.Name == "api" && r.CreatedByRun }) {
		t.Errorf("journal lost the created app: %v %s", err, data)
	}
}
