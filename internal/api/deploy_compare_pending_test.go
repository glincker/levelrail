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

func TestHandleCompareDeploys_Pending(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:2", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	base := time.Now().UTC().Truncate(time.Millisecond)
	for i, a := range []store.DeployAttempt{
		{ID: "dep_1", Image: "levelrail/web:1", Status: store.DeployAttemptStatusSucceeded, StartedAt: base,
			Snapshot: store.DeployAttemptSnapshot{Port: 3000, Env: []store.DeployAttemptEnvKey{{Key: "TOKEN", Kind: store.DeployAttemptEnvKindLiteral, Value: "hunter2"}}}},
		{ID: "dep_2", Image: "levelrail/web:bad", Status: store.DeployAttemptStatusFailed, StartedAt: base.Add(time.Minute)},
	} {
		a.ServiceName = "web"
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatalf("seed attempt %d: %v", i, err)
		}
	}

	tests := []struct {
		name   string
		query  string
		wantTo string
	}{
		{"defaults to the live deploy", "pending=true", "levelrail/web:2"},
		{"image override", "pending=true&image=levelrail/web:9", "levelrail/web:9"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/compare?"+tc.query, ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
			}
			var got deployCompareResource
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.From.DeployID != "dep_1" {
				t.Errorf("from = %q, want the last succeeded attempt dep_1", got.From.DeployID)
			}
			if got.To.Image != tc.wantTo {
				t.Errorf("to image = %q, want %q", got.To.Image, tc.wantTo)
			}
			for _, e := range got.From.Env {
				if e.Value != "" {
					t.Errorf("env %q value leaked in a pending diff", e.Key)
				}
			}
			for _, e := range got.EnvChanges {
				if e.From != "" || e.To != "" {
					t.Errorf("env change %q carries a value", e.Key)
				}
			}
		})
	}
}

func TestHandleCompareDeploys_PendingWithoutHistory(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/compare?pending=true", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
}
