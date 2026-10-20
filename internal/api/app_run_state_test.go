package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// fakeExitRuntime reports exit state for one container and serves its logs.
type fakeExitRuntime struct {
	fakeLogsRuntime
	exit *docker.ExitState
}

func (f *fakeExitRuntime) InspectExitState(context.Context, string) (*docker.ExitState, error) {
	if f.exit == nil {
		return nil, nil
	}
	cp := *f.exit
	return &cp, nil
}

const runStateImage = "levelrail/web:1"

func runStateTarget() string {
	return application.ContainerName("web", runStateImage, "")
}

func seedRunStateApp(t *testing.T, db *store.DB) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: runStateImage, Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_exit", ServiceName: "web", Image: runStateImage, Source: store.DeployAttemptSourceCompose,
		Status: store.DeployAttemptStatusFailed, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
}

func recordRunStateExit(t *testing.T, db *store.DB, container string) {
	t.Helper()
	now := time.Now().UTC()
	if err := db.RecordDeployExit(context.Background(), store.DeployExit{
		ServiceName: "web", Image: runStateImage, Container: container, ExitCode: 0,
		Reason: "ExitedDuringReadiness", Message: "Container exited with code 0 after 2s, before it became ready",
		StartedAt: now.Add(-2 * time.Second), FinishedAt: now,
		LogTail: []store.DeployExitLine{{Timestamp: now, Stream: "stdout", Message: "usage: web serve"}},
	}); err != nil {
		t.Fatalf("record exit: %v", err)
	}
}

func TestAppRunState(t *testing.T) {
	finished := time.Now().UTC()
	tests := []struct {
		name        string
		runtime     docker.Runtime
		capture     string
		wantKnown   bool
		wantRunning bool
		wantMsg     string
	}{
		{name: "runtime cannot inspect exits", runtime: &fakeExecAppRuntime{}},
		{name: "running", runtime: &fakeExitRuntime{exit: &docker.ExitState{Running: true}}, wantKnown: true, wantRunning: true},
		{
			name:      "exited, no capture, message from exit state",
			runtime:   &fakeExitRuntime{exit: &docker.ExitState{ExitCode: 3, StartedAt: finished.Add(-5 * time.Second), FinishedAt: finished}},
			wantKnown: true, wantMsg: "Container exited with code 3 after 5s, before it became ready",
		},
		{
			name:    "exited with matching capture",
			runtime: &fakeExitRuntime{exit: &docker.ExitState{ExitCode: 0, FinishedAt: finished}}, capture: runStateTarget(),
			wantKnown: true, wantMsg: "Container exited with code 0 after 2s, before it became ready",
		},
		{
			name:    "container gone, capture still explains",
			runtime: &fakeExitRuntime{}, capture: runStateTarget(),
			wantKnown: true, wantMsg: "Container exited with code 0 after 2s, before it became ready",
		},
		{name: "container gone, nothing captured", runtime: &fakeExitRuntime{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(func(string) (docker.Runtime, error) { return tt.runtime, nil }))
			seedRunStateApp(t, db)
			if tt.capture != "" {
				recordRunStateExit(t, db, tt.capture)
			}
			st, ok := rt.appRunState(context.Background(), "web")
			if ok != tt.wantKnown {
				t.Fatalf("known = %v, want %v (%+v)", ok, tt.wantKnown, st)
			}
			if st.Running != tt.wantRunning || st.Message != tt.wantMsg {
				t.Fatalf("status = %+v, want running=%v message %q", st, tt.wantRunning, tt.wantMsg)
			}
			if ok && !st.Running && st.ExitCode == nil {
				t.Fatal("stopped status has no exit code")
			}
		})
	}
}

func TestHandleGetDeploy_IncludesExitCapture(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedRunStateApp(t, db)
	recordRunStateExit(t, db, runStateTarget())

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_exit", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got deployAttemptResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Exit == nil || got.Exit.ExitCode != 0 || got.Exit.Container != runStateTarget() || len(got.Exit.Lines) != 1 {
		t.Fatalf("exit = %+v, want the capture with its lines", got.Exit)
	}
}

func TestHandleFailureContext_FallsBackToExitCapture(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedRunStateApp(t, db)
	recordRunStateExit(t, db, runStateTarget())

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/failure-context", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got failureContextResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.State != failureStateDeployFailed || got.LinesSource != "exit" || len(got.Lines) != 1 || got.Lines[0].Message != "usage: web serve" {
		t.Fatalf("failure context = %+v, want the exit capture's lines", got)
	}
	if got.Exit == nil || got.Exit.Message == "" || len(got.Exit.Lines) != 0 {
		t.Fatalf("exit = %+v, want the exit without duplicated lines", got.Exit)
	}
}

// readSSEUntil reads raw SSE lines until one contains want.
func readSSEUntil(t *testing.T, r *bufio.Reader, want string) []string {
	t.Helper()
	var seen []string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read stream (seen %q): %v", seen, err)
		}
		line = strings.TrimRight(line, "\r\n")
		seen = append(seen, line)
		if strings.Contains(line, want) {
			return seen
		}
	}
}

func TestHandleLiveLogStream_StatusEvents(t *testing.T) {
	tests := []struct {
		name       string
		exit       *docker.ExitState
		wantStatus string
		wantLine   string
	}{
		{
			name:       "stopped container sends status then its last output",
			exit:       &docker.ExitState{ExitCode: 0, FinishedAt: time.Now().UTC()},
			wantStatus: `"running":false`,
			wantLine:   `"line":"printed help and exited"`,
		},
		{
			name:       "running container sends running status then live lines",
			exit:       &docker.ExitState{Running: true},
			wantStatus: `"running":true`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			tdb := newTestTelemetryDB(t)
			broadcaster := telemetry.NewLogBroadcaster()
			runtime := &fakeExitRuntime{fakeLogsRuntime: fakeLogsRuntime{lines: []string{"printed help and exited"}}, exit: tt.exit}
			rt := NewRouter(discardLogger(), testBrand(), db,
				WithTelemetryQuerier(telemetry.NewLocalFederator(tdb)), WithLogBroadcaster(broadcaster),
				WithExecRuntime(func(string) (docker.Runtime, error) { return runtime, nil }))
			seedRunStateApp(t, db)
			srv := httptest.NewServer(rt.Handler())
			defer srv.Close()
			cookie := loginViaServer(t, srv, db)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/apps/web/logs/stream", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.AddCookie(cookie)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			reader := bufio.NewReader(resp.Body)

			seen := readSSEUntil(t, reader, "event: status")
			if seen[len(seen)-1] != "event: status" {
				t.Fatalf("status line = %q", seen[len(seen)-1])
			}
			data, err := nextSSEData(reader)
			if err != nil || !strings.Contains(data, tt.wantStatus) {
				t.Fatalf("status data = %q, %v; want %s", data, err, tt.wantStatus)
			}
			if tt.wantLine != "" {
				readSSEUntil(t, reader, tt.wantLine)
				return
			}
			time.Sleep(20 * time.Millisecond)
			broadcaster.Publish(telemetry.LogEntry{ResourceID: "service:web", Stream: "stdout", Message: "live line"})
			if line, err := nextSSEData(reader); err != nil || !strings.Contains(line, "live line") {
				t.Fatalf("live line = %q, %v; want streaming to continue", line, err)
			}
		})
	}
}
