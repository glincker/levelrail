package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestNextBuildReport(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	logs := &fakeDeployLogQuerier{entries: map[string][]sseTestEntry{
		"dep_n": {
			{Message: "Route (app)   Size  First Load JS", Stream: "stdout"},
			{Message: "┌ ○ /   5.4 kB   92.5 kB", Stream: "stdout"},
			{Message: "└ ƒ /api/x   0 B   0 B", Stream: "stdout"},
			{Message: "+ First Load JS shared by all   87.1 kB", Stream: "stdout"},
			{Message: " ⚠ something is off", Stream: "stderr"},
		},
		"dep_plain": {{Message: "go build", Stream: "stdout"}},
	}}
	rt := NewRouter(nil, testBrand(), db, WithDeployLogQuerier(logs))
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"dep_n", "dep_plain"} {
		if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: id, ServiceName: "web", Image: "levelrail/web:2", Status: store.DeployAttemptStatusRunning, StartedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		if err := db.FinishDeployAttempt(ctx, id, store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
			t.Fatal(err)
		}
	}
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, path, ""))
		return rec
	}

	rec := get("/api/v1/apps/web/deploys/dep_n/build-report")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Detected bool `json:"detected"`
		Routes   []struct {
			Path        string  `json:"path"`
			Kind        string  `json:"kind"`
			SizeKB      float64 `json:"size_kb"`
			FirstLoadKB float64 `json:"first_load_kb"`
		} `json:"routes"`
		Shared   *float64 `json:"shared_first_load_kb"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Detected || len(got.Routes) != 2 || got.Routes[1].Kind != "dynamic" || got.Shared == nil || *got.Shared != 87.1 || len(got.Warnings) != 1 {
		t.Fatalf("report = %s", rec.Body.String())
	}

	rec = get("/api/v1/apps/web/deploys/dep_plain/build-report")
	if rec.Code != http.StatusOK || !jsonHas(rec.Body.String(), `"detected":false`, `"routes":[]`, `"shared_first_load_kb":null`, `"warnings":[]`) {
		t.Fatalf("plain = %d %s", rec.Code, rec.Body.String())
	}
	if rec := get("/api/v1/apps/web/deploys/missing/build-report"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing attempt status = %d", rec.Code)
	}
	if rec := get("/api/v1/apps/nope/deploys/dep_n/build-report"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing app status = %d", rec.Code)
	}
}

func jsonHas(body string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(body, p) {
			return false
		}
	}
	return true
}
