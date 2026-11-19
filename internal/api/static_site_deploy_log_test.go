package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type fakeDeployLogs struct{ entries []telemetry.DeployLogEntry }

func (f fakeDeployLogs) QueryDeployLog(context.Context, string) ([]telemetry.DeployLogEntry, error) {
	return f.entries, nil
}

func TestStaticSiteDeployLog_MasksAndScopes(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, inspectResolver{})
	seedApp(t, db, "web")
	seedApp(t, db, "other")
	if err := db.SaveStaticSite(context.Background(), store.StaticSite{Name: "web", Domains: []string{"web.example.com"}, RootDir: t.TempDir() + "/web/x"}); err != nil {
		t.Fatal(err)
	}
	done := time.Now()
	seedTarget(t, db, store.DeployAttempt{ID: "dep_log1", Image: "web:a", FinishedAt: &done})
	seedTarget(t, db, store.DeployAttempt{ID: "dep_foreign", ServiceName: "other", Image: "o:a", FinishedAt: &done})
	secretmask.Add("sup3r-secret-value-123")
	rt.deployLogStore = fakeDeployLogs{entries: []telemetry.DeployLogEntry{
		{Stream: "stdout", Timestamp: time.Now(), Message: "npm ci"},
		{Stream: "stderr", Timestamp: time.Now(), Message: "token sup3r-secret-value-123 leaked"},
	}}

	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/static-sites/web/deploys/dep_log1/log", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("log = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sup3r-secret-value-123") {
		t.Fatalf("secret leaked: %s", rec.Body.String())
	}
	var got staticDeployLogResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got.Lines) != 2 {
		t.Fatalf("response = %+v err=%v", got, err)
	}

	rec = serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/static-sites/web/deploys/dep_log1/log?tail=1", ""))
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got.Lines) != 1 {
		t.Fatalf("tail = %+v err=%v", got, err)
	}
	if rec = serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/static-sites/web/deploys/dep_foreign/log", "")); rec.Code != http.StatusNotFound {
		t.Fatalf("foreign deploy = %d, want 404", rec.Code)
	}
}
