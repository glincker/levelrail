package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDeploymentsTimelineWindowAndFilters(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	now := time.Now().UTC()
	for i, app := range []string{"web", "api"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: app, Image: app + ":1", Port: 80}); err != nil {
			t.Fatal(err)
		}
		// api started 3 days ago, outside the 24h default window.
		started := now.Add(-time.Duration(1+i*72) * time.Hour)
		if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
			ID: "dep_" + app, ServiceName: app, Image: app + ":1", Source: store.DeployAttemptSourceImage,
			Status: store.DeployAttemptStatusFailed, StartedAt: started,
		}); err != nil {
			t.Fatal(err)
		}
	}
	get := func(query string) (int, fleetTimelineResponse) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/timeline"+query, nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		var out fleetTimelineResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	tests := []struct {
		name     string
		query    string
		wantCode int
		wantApps int
	}{
		{"default window is 24h", "", http.StatusOK, 1},
		{"seven day window", "?since=7d", http.StatusOK, 2},
		{"app filter", "?since=7d&app=api", http.StatusOK, 1},
		{"unknown project is empty", "?since=7d&project=nope", http.StatusOK, 0},
		{"window over 30d rejected", "?since=45d", http.StatusBadRequest, 0},
		{"bad status rejected", "?status=nope", http.StatusBadRequest, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out := get(tt.query)
			if code != tt.wantCode {
				t.Fatalf("status = %d, want %d", code, tt.wantCode)
			}
			if code == http.StatusOK && len(out.Items) != tt.wantApps {
				t.Fatalf("items = %d, want %d", len(out.Items), tt.wantApps)
			}
		})
	}
}
