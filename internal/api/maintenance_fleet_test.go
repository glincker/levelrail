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

func TestMaintenanceAutoOffFleetAndPreview(t *testing.T) {
	t.Setenv(envMaintenanceMaxDuration, "2h")
	t.Setenv(envMaintenanceAttentionAge, "1h")
	rt, db := newTestRouter(t)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"}}); err != nil {
		t.Fatal(err)
	}
	cookie := loginTestSession(t, rt, db)

	code, first := maintenanceCall(t, rt, cookie, http.MethodPut, `{}`)
	if code != http.StatusOK || first.StartedAt == "" || first.AutoOffAt == "" {
		t.Fatalf("env default not applied: %d %+v", code, first)
	}
	// Saving again keeps the original start.
	code, again := maintenanceCall(t, rt, cookie, http.MethodPut, `{"max_duration_seconds":0,"title":"x"}`)
	if code != http.StatusOK || again.StartedAt != first.StartedAt || again.AutoOffAt != "" {
		t.Fatalf("second save: %d %+v", code, again)
	}
	if code, _ = maintenanceCall(t, rt, cookie, http.MethodPut, `{"max_duration_seconds":-5}`); code != http.StatusBadRequest {
		t.Fatalf("negative duration = %d", code)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/maintenance/domains", ""))
	var fleet fleetMaintenanceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &fleet); err != nil || len(fleet.Items) != 1 || fleet.Items[0].App != "web" ||
		fleet.MaxDurationSeconds != 7200 || fleet.AttentionAfterSeconds != 3600 {
		t.Fatalf("fleet = %s (%v)", rec.Body.String(), err)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/maintenance/preview", `{"title":"Back at 5","message":"a {b}"}`))
	var pv maintenancePreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &pv); err != nil || pv.Status != 503 || !strings.Contains(pv.HTML, "Back at 5") || !strings.Contains(pv.HTML, "a {b}") || pv.RetryAfterSeconds != "300" {
		t.Fatalf("preview = %s (%v)", rec.Body.String(), err)
	}

	items := rt.maintenanceAttentionItems(httptest.NewRequest(http.MethodGet, "/", nil), []string{AbilityRead}, time.Now().Add(3*time.Hour))
	if len(items) != 1 || items[0].Kind != "maintenance_long" {
		t.Fatalf("attention = %+v", items)
	}
	if got := rt.maintenanceAttentionItems(httptest.NewRequest(http.MethodGet, "/", nil), []string{AbilityRead}, time.Now()); len(got) != 0 {
		t.Fatalf("fresh maintenance raised attention: %+v", got)
	}
}
