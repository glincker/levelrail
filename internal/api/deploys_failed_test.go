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

func TestHandleListFailedDeploys(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	now := time.Now().UTC()
	for _, a := range []store.DeployAttempt{
		{ID: "dep_1", ServiceName: "web", Image: "levelrail/web:1", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusSucceeded, StartedAt: now.Add(-3 * time.Hour)},
		{ID: "dep_2", ServiceName: "web", Image: "levelrail/web:2", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusFailed, StartedAt: now.Add(-time.Hour), Error: "boom"},
	} {
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatalf("seed attempt: %v", err)
		}
	}

	if err := db.FinishDeployAttempt(ctx, "dep_2", store.DeployAttemptStatusFailed, now, "boom"); err != nil {
		t.Fatalf("finish attempt: %v", err)
	}

	tests := []struct {
		name, query string
		wantCode    int
		wantRows    int
	}{
		{"default window", "", http.StatusOK, 1},
		{"narrow window", "?since=30m", http.StatusOK, 0},
		{"bad duration", "?since=nope", http.StatusBadRequest, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/deploys/failed"+tt.query, ""))
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantCode != http.StatusOK {
				return
			}
			var got []failedDeployResource
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(got) != tt.wantRows {
				t.Fatalf("got %d rows, want %d", len(got), tt.wantRows)
			}
			if tt.wantRows == 1 && (got[0].ServiceName != "web" || got[0].Error != "boom" || got[0].LastGoodImage != "levelrail/web:1") {
				t.Errorf("row = %+v", got[0])
			}
		})
	}
}
