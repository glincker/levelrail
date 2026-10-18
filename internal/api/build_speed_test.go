package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestBuildSpeed(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithBuildPerformance(db))
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	rec := BuildRunRecorder(db)
	for i := 0; i < 3; i++ {
		if err := rec.RecordBuildRun(ctx, build.Run{App: "web", Tag: "web:x", StartedAt: time.Unix(int64(i), 0), Duration: time.Minute,
			Status: build.RunSuccess, CachedSteps: 0, TotalSteps: 5}); err != nil {
			t.Fatal(err)
		}
	}
	do := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		rt.Handler().ServeHTTP(w, authedRequest(t, cookie, http.MethodGet, path, ""))
		return w
	}
	w := do("/api/v1/apps/web/build-speed")
	if w.Code != http.StatusOK {
		t.Fatalf("speed %d: %s", w.Code, w.Body)
	}
	var rep build.SpeedReport
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Builds != 3 || len(rep.Findings) != 2 || rep.Findings[0].Code != build.SpeedCacheUnconfigured {
		t.Fatalf("report = %+v", rep)
	}
	if w = do("/api/v1/apps/ghost/build-speed"); w.Code != http.StatusNotFound {
		t.Fatalf("unknown app: %d", w.Code)
	}
}
