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

func maintenanceCall(t *testing.T, rt *Router, cookie *http.Cookie, method, body string) (int, domainMaintenanceResource) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, "/api/v1/apps/web/domains/app.example.com/maintenance", body))
	var res domainMaintenanceResource
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatalf("decode: %v (%s)", err, rec.Body.String())
		}
	}
	return rec.Code, res
}

func TestDomainMaintenance_OptionsWindowAndClear(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.SetScheduledMaintenance(db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"}}); err != nil {
		t.Fatal(err)
	}
	cookie := loginTestSession(t, rt, db)
	until := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339)

	code, res := maintenanceCall(t, rt, cookie, http.MethodPut, `{"title":"Upgrading","message":"Back soon","retry_after_seconds":120,"until":"`+until+`","bypass":["198.51.100.7","10.0.0.0/8"],"bypass_health_check":false}`)
	if code != http.StatusOK || !res.Enabled || res.Title != "Upgrading" || res.RetryAfterSeconds != 120 || res.Until != until ||
		res.WindowID == "" || res.BypassHealthCheck || strings.Join(res.Bypass, ",") != "198.51.100.7/32,10.0.0.0/8" {
		t.Fatalf("PUT = %d %+v", code, res)
	}
	windows, err := db.ListMaintenanceWindows(ctx, time.Now().Add(-time.Hour))
	if err != nil || len(windows) != 1 || windows[0].ID != res.WindowID || windows[0].Target != "web" || windows[0].Freeze {
		t.Fatalf("linked window = %+v, %v", windows, err)
	}

	code, got := maintenanceCall(t, rt, cookie, http.MethodGet, "")
	if code != http.StatusOK || got.Until != until || got.Message != "Back soon" {
		t.Errorf("GET = %d %+v", code, got)
	}

	// Replacing the settings without an end time drops the old window.
	if code, res = maintenanceCall(t, rt, cookie, http.MethodPut, `{"title":"Still upgrading"}`); code != http.StatusOK || res.WindowID != "" || !res.BypassHealthCheck {
		t.Fatalf("replace = %d %+v", code, res)
	}
	if windows, _ = db.ListMaintenanceWindows(ctx, time.Now().Add(-time.Hour)); len(windows) != 0 {
		t.Errorf("old window kept: %+v", windows)
	}

	if code, res = maintenanceCall(t, rt, cookie, http.MethodPut, `{"until":"`+until+`"}`); code != http.StatusOK || res.WindowID == "" {
		t.Fatalf("timed = %d %+v", code, res)
	}
	if code, res = maintenanceCall(t, rt, cookie, http.MethodDelete, ""); code != http.StatusOK || res.Enabled {
		t.Fatalf("DELETE = %d %+v", code, res)
	}
	if windows, _ = db.ListMaintenanceWindows(ctx, time.Now().Add(-time.Hour)); len(windows) != 0 {
		t.Errorf("clearing kept the window: %+v", windows)
	}
}

func TestDomainMaintenance_RejectsBadInput(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.SetScheduledMaintenance(db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"}}); err != nil {
		t.Fatal(err)
	}
	cookie := loginTestSession(t, rt, db)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	for _, body := range []string{
		`{"bypass":["0.0.0.0/0"]}`,
		`{"bypass":["nope"]}`,
		`{"until":"` + past + `"}`,
		`{"retry_after_seconds":-5}`,
		`{"title":"` + strings.Repeat("x", 121) + `"}`,
		`not json`,
	} {
		if code, _ := maintenanceCall(t, rt, cookie, http.MethodPut, body); code != http.StatusBadRequest {
			t.Errorf("PUT %s = %d, want 400", body, code)
		}
	}
	if code, res := maintenanceCall(t, rt, cookie, http.MethodGet, ""); code != http.StatusOK || res.Enabled {
		t.Errorf("rejected input still enabled maintenance: %+v", res)
	}
}
