package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/janitor"
)

type janitorTestDocker struct{ janitor.Docker }

func (janitorTestDocker) DiskUsage(context.Context) (docker.DiskUsage, error) {
	return docker.DiskUsage{ContainersReclaimableBytes: 7}, nil
}

func (janitorTestDocker) ListAllImages(context.Context) ([]docker.ImageRecord, error) {
	return []docker.ImageRecord{{ID: "sha256:abcdef0123456789", SizeBytes: 3}}, nil
}

func (janitorTestDocker) PruneContainers(context.Context, []string) (docker.PruneContainersResult, error) {
	return docker.PruneContainersResult{}, nil
}

func (janitorTestDocker) PruneDanglingImages(context.Context) (docker.PruneImagesResult, error) {
	return docker.PruneImagesResult{}, nil
}

func (janitorTestDocker) PruneAnonymousVolumes(context.Context) (docker.PruneVolumesResult, error) {
	return docker.PruneVolumesResult{}, nil
}

func (janitorTestDocker) PruneBuildCacheOlderThan(context.Context, time.Duration) (docker.PruneBuildCacheResult, error) {
	return docker.PruneBuildCacheResult{}, nil
}

func (janitorTestDocker) BuildCacheRecords(context.Context) ([]docker.BuildCacheRecord, error) {
	return nil, nil
}

func TestJanitorEndpoints(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	do := func(method, target, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, body))
		return rec
	}

	if rec := do(http.MethodGet, "/api/v1/system/janitor", ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("unconfigured status = %d, want 501", rec.Code)
	}

	rt.NewJanitor(db, janitorTestDocker{})

	rec := do(http.MethodGet, "/api/v1/system/janitor", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var st janitorStatusResource
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if !st.Policy.DryRun || !st.Policy.Enabled {
		t.Fatalf("default policy must preview only: %+v", st.Policy)
	}

	if rec := do(http.MethodPut, "/api/v1/system/janitor/policy", `{"interval_minutes":1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid policy status = %d, want 400", rec.Code)
	}
	if rec := do(http.MethodPut, "/api/v1/system/janitor/policy", `{"attempt_keep_per_app":5}`); rec.Code != http.StatusOK {
		t.Fatalf("policy update status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = do(http.MethodPost, "/api/v1/system/janitor/run?dry_run=true", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("run status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var rep janitor.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun || len(rep.Stages) != len(janitor.AllStages) {
		t.Fatalf("report = %+v", rep)
	}

	rec = do(http.MethodPost, "/api/v1/system/janitor/run?force=false", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("real run status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}

	rec = do(http.MethodGet, "/api/v1/system/janitor/runs", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), rep.ID) {
		t.Fatalf("runs status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
