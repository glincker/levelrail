package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
)

func TestScheduledMaintenanceLifecycle(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.SetScheduledMaintenance(db)
	cookie := loginTestSession(t, rt, db)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	start := time.Now().UTC().Add(2 * time.Hour)
	body, _ := json.Marshal(map[string]any{"title": "Disk swap", "scope": "all", "freeze": true,
		"starts_at": start, "ends_at": start.Add(time.Hour), "announce_lead_minutes": 30})
	rec := do(http.MethodPost, "/api/v1/scheduled-maintenance", string(body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	var created scheduledMaintenanceResource
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.State != "scheduled" || !created.Freeze {
		t.Fatalf("created = %+v", created)
	}

	rec = do(http.MethodGet, "/api/v1/scheduled-maintenance?current=true", "")
	var current []scheduledMaintenanceResource
	_ = json.Unmarshal(rec.Body.Bytes(), &current)
	if len(current) != 1 {
		t.Fatalf("banner list = %d windows, want 1 (starts inside the 24h floor)", len(current))
	}

	// The window starts in 2h, so it is not frozen yet.
	st, err := deploy.CheckFreeze(t.Context(), db, "web", time.Now())
	if err != nil || st.Frozen {
		t.Fatalf("freeze before start = %+v, %v", st, err)
	}
	st, err = deploy.CheckFreeze(t.Context(), db, "web", start.Add(time.Minute))
	if err != nil || !st.Frozen || !strings.Contains(st.Reason, "Disk swap") {
		t.Fatalf("freeze during window = %+v, %v", st, err)
	}
	st, _ = deploy.CheckFreeze(t.Context(), db, "web", start.Add(61*time.Minute))
	if st.Frozen {
		t.Fatal("window still freezing after its end")
	}

	bad, _ := json.Marshal(map[string]any{"title": "x", "scope": "app", "starts_at": start, "ends_at": start.Add(time.Hour)})
	if rec = do(http.MethodPost, "/api/v1/scheduled-maintenance", string(bad)); rec.Code != http.StatusBadRequest {
		t.Fatalf("app scope without target = %d, want 400", rec.Code)
	}

	if rec = do(http.MethodDelete, "/api/v1/scheduled-maintenance/"+created.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec = do(http.MethodDelete, "/api/v1/scheduled-maintenance/"+created.ID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d, want 404", rec.Code)
	}
}
