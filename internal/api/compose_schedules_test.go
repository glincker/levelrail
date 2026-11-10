package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/scheduledtask"
	"github.com/GLINCKER/levelrail/internal/store"
)

const composeScheduleStack = "services:\n  web:\n    image: nginx:1.27\n  worker:\n    image: busybox:1\n"

// exitReader prints out, then fails like a command that exited with code.
type exitReader struct {
	r    io.Reader
	code int
}

func (e *exitReader) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if err == io.EOF {
		return n, &docker.ExecExitError{ExitCode: e.code, Stderr: "boom"}
	}
	return n, err
}

func (e *exitReader) Close() error { return nil }

func TestComposeScheduleRunsInNamedServiceAndRecordsHistory(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	fake := &fakeExecAppRuntime{
		inspectState: &docker.ContainerState{ID: "ctr-web", Running: true},
		execReader:   &exitReader{r: strings.NewReader("hello from web\n"), code: 3},
	}
	runner := &scheduledtask.Runner{
		Apps: db, Runs: db, History: db, Limits: scheduledtask.DefaultLimits(),
		Resolver: func(string) (docker.Runtime, error) { return fake, nil },
	}
	rt := NewRouter(discardLogger(), testBrand(), db, WithScheduledTaskRunner(runner))
	cookie := loginTestSession(t, rt, db)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/stack/compose", composeScheduleStack, http.StatusOK)
	composeDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/stack-web/exec-access", `{"enabled":true}`, http.StatusOK)

	base := "/api/v1/apps/stack/compose/services/web/schedules"
	var created scheduledTaskResource
	body := composeDo(t, rt, cookie, http.MethodPost, base, `{"name":"report","command":["sh","-c","echo hi"],"schedule":"@hourly"}`, http.StatusCreated)
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.ServiceName != "stack-web" || created.Mode != store.ScheduledTaskModeExec || !created.Enabled {
		t.Fatalf("created = %+v, want an enabled exec job on stack-web", created)
	}

	var started runNowResponse
	if err := json.Unmarshal(composeDo(t, rt, cookie, http.MethodPost, base+"/"+created.ID+"/run", "", http.StatusAccepted), &started); err != nil || started.RunID == "" {
		t.Fatalf("run now: %v, run id %q", err, started.RunID)
	}
	var run scheduledTaskRunResource
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := json.Unmarshal(composeDo(t, rt, cookie, http.MethodGet, base+"/"+created.ID+"/runs/"+started.RunID, "", http.StatusOK), &run); err != nil {
			t.Fatalf("decode run: %v", err)
		}
		if run.Status != store.ScheduledTaskStatusRunning || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if run.Status != store.ScheduledTaskStatusFailed || run.ExitCode == nil || *run.ExitCode != 3 {
		t.Fatalf("run = %+v, want failed with exit code 3", run)
	}
	if !strings.Contains(run.Output, "hello from web") || !strings.Contains(run.Output, "boom") {
		t.Fatalf("output = %q, want stdout and stderr", run.Output)
	}
	if fake.gotContainerID != "ctr-web" || strings.Join(fake.gotCmd, " ") != "sh -c echo hi" {
		t.Fatalf("exec target = %q %v, want the web container and the job command", fake.gotContainerID, fake.gotCmd)
	}

	var list struct {
		Schedules []composeScheduleResource `json:"schedules"`
	}
	if err := json.Unmarshal(composeDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/stack/compose/schedules", "", http.StatusOK), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Schedules) != 1 || list.Schedules[0].Service != "web" {
		t.Fatalf("schedules = %+v, want one on web", list.Schedules)
	}

	for _, path := range []string{"/api/v1/apps/nostack/compose/services/web/schedules", "/api/v1/apps/stack/compose/services/nope/schedules", "/api/v1/apps/nostack/compose/schedules"} {
		got := composeDo(t, rt, cookie, http.MethodGet, path, "", http.StatusNotFound)
		if !strings.Contains(string(got), composeScheduleNotFound) {
			t.Errorf("%s body = %s, want %q", path, got, composeScheduleNotFound)
		}
	}
}

func TestComposeScheduleAccessRules(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/stack/compose", composeScheduleStack, http.StatusOK)
	base := "/api/v1/apps/stack/compose/services/web/schedules"
	job := `{"name":"report","command":["true"],"schedule":"@hourly"}`

	// With console exec off for stack-web, a schedule follows the console rule.
	composeDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/stack-web/exec-access", `{"enabled":false}`, http.StatusOK)
	composeDo(t, rt, cookie, http.MethodPost, base, job, http.StatusForbidden)

	tok := seedMatrixToken(t, db, "ro-tok", []string{AbilityRead})
	do := func(method, path, body string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		bearer(tok)(req)
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if code := do(http.MethodPost, base, job); code != http.StatusForbidden {
		t.Errorf("read-only create = %d, want 403", code)
	}
	if code := do(http.MethodPost, base+"/sct_x/run", ""); code != http.StatusForbidden {
		t.Errorf("read-only run = %d, want 403", code)
	}
	if code := do(http.MethodGet, base, ""); code != http.StatusOK {
		t.Errorf("read-only list = %d, want 200", code)
	}
}

func TestComposeScheduleOtherTeamStackIsNotFound(t *testing.T) {
	f := newOrgFixture(t)
	ctx := context.Background()
	seedStackAttempt(t, f.db, "shopb")
	for _, m := range []string{"shopb-web", "shopb-db"} {
		if err := f.db.UpdateServiceProject(ctx, m, projB); err != nil {
			t.Fatal(err)
		}
		if err := f.db.SetServiceEnvironment(ctx, m, envB); err != nil {
			t.Fatal(err)
		}
	}
	ownerA := withCookie(f.cookies[store.TeamRoleOwner])
	for _, path := range []string{"/api/v1/apps/shopb/compose/services/web/schedules", "/api/v1/apps/shopb/compose/schedules"} {
		code, body := teamDo(t, f.rt, http.MethodGet, path, "", ownerA)
		if code != http.StatusNotFound || !strings.Contains(body, composeScheduleNotFound) {
			t.Errorf("%s = %d %s, want the unknown-name 404", path, code, body)
		}
	}
}
