package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeLogsRuntime struct {
	fakeExecAppRuntime
	lines  []string
	err    error
	gotID  string
	gotFol bool
}

func (f *fakeLogsRuntime) Logs(_ context.Context, containerID string, follow bool, _ time.Time) (<-chan docker.LogLine, <-chan error) {
	f.gotID, f.gotFol = containerID, follow
	out := make(chan docker.LogLine, len(f.lines))
	errs := make(chan error, 1)
	for _, l := range f.lines {
		out <- docker.LogLine{Stream: "stderr", Message: l}
	}
	if f.err != nil {
		errs <- f.err
	}
	close(out)
	close(errs)
	return out, errs
}

func TestHandleDiagnoseApp_ContainerLogFallback(t *testing.T) {
	tests := []struct {
		name        string
		runtime     docker.Runtime
		wantCode    string
		wantExcerpt string
	}{
		{
			name:        "empty log store falls back to container output",
			runtime:     &fakeLogsRuntime{lines: []string{"booting", "missing required env GREETING"}},
			wantCode:    "missing_env",
			wantExcerpt: "missing required env GREETING",
		},
		{
			name:     "log read error leaves the failure unclassified by logs",
			runtime:  &fakeLogsRuntime{err: errors.New("no such container")},
			wantCode: "unknown",
		},
		{
			name:     "runtime without log access is skipped",
			runtime:  &fakeExecAppRuntime{},
			wantCode: "unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(func(string) (docker.Runtime, error) { return tt.runtime, nil }))
			cookie := loginTestSession(t, rt, db)
			ctx := context.Background()

			svc := store.DesiredService{Name: "web", Image: "levelrail/web:1", ImageID: "sha256:abc", ImageIDRef: "levelrail/web:1", Port: 3000}
			if err := db.SaveDesiredService(ctx, svc); err != nil {
				t.Fatalf("seed app: %v", err)
			}
			base := time.Now().UTC().Truncate(time.Millisecond)
			if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
				ID: "dep_crash", ServiceName: "web", Image: "levelrail/web:1",
				Source: store.DeployAttemptSourceManual, Status: store.DeployAttemptStatusRunning, StartedAt: base,
			}); err != nil {
				t.Fatalf("seed attempt: %v", err)
			}
			if err := db.FinishDeployAttempt(ctx, "dep_crash", store.DeployAttemptStatusFailed, base.Add(time.Second), "rollout did not become ready"); err != nil {
				t.Fatalf("finish attempt: %v", err)
			}

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/diagnose", ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			var got diagnosisResource
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.Failure == nil {
				t.Fatalf("Failure = nil, want code %q", tt.wantCode)
			}
			if got.Failure.Code != tt.wantCode {
				t.Errorf("Failure.Code = %q, want %q; failure %+v", got.Failure.Code, tt.wantCode, got.Failure)
			}
			if !strings.Contains(got.Failure.LogExcerpt, tt.wantExcerpt) {
				t.Errorf("Failure.LogExcerpt = %q, want it to contain %q", got.Failure.LogExcerpt, tt.wantExcerpt)
			}
			want := application.ContainerName("web", application.NameImage(svc), "")
			if f, ok := tt.runtime.(*fakeLogsRuntime); ok && (f.gotFol || f.gotID != want) {
				t.Errorf("Logs(id %q, follow %v), want a one-shot read of %q", f.gotID, f.gotFol, want)
			}
		})
	}
}
