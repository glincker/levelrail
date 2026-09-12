package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleRestartProject(t *testing.T) {
	var webNonceBefore string

	cases := []struct {
		name       string
		projectID  string
		setup      func(t *testing.T, ctx context.Context, db *store.DB)
		wantStatus int
		check      func(t *testing.T, ctx context.Context, db *store.DB, got projectRestartResponse)
	}{
		{
			name:      "success",
			projectID: "proj_1",
			setup: func(t *testing.T, ctx context.Context, db *store.DB) {
				if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "my-saas"}); err != nil {
					t.Fatalf("SaveProject() error = %v", err)
				}
				for _, svc := range []store.DesiredService{
					{Name: "web", Image: "levelrail/web:1", Port: 3000},
					{Name: "worker", Image: "levelrail/worker:1", Port: 3001},
					{Name: "unrelated", Image: "levelrail/unrelated:1", Port: 3002},
				} {
					if err := db.SaveDesiredService(ctx, svc); err != nil {
						t.Fatalf("SaveDesiredService(%s) error = %v", svc.Name, err)
					}
				}
				if err := db.UpdateServiceProject(ctx, "web", "proj_1"); err != nil {
					t.Fatalf("UpdateServiceProject(web) error = %v", err)
				}
				if err := db.UpdateServiceProject(ctx, "worker", "proj_1"); err != nil {
					t.Fatalf("UpdateServiceProject(worker) error = %v", err)
				}

				before, err := db.GetDesiredService(ctx, "web")
				if err != nil {
					t.Fatalf("GetDesiredService(web) error = %v", err)
				}
				webNonceBefore = before.RestartNonce
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, ctx context.Context, db *store.DB, got projectRestartResponse) {
				if got.RestartedCount != 2 {
					t.Errorf("RestartedCount = %d, want 2", got.RestartedCount)
				}
				if len(got.Apps) != 2 || got.Apps[0] != "web" || got.Apps[1] != "worker" {
					t.Errorf("Apps = %+v, want [web worker]", got.Apps)
				}
				if len(got.Failed) != 0 {
					t.Errorf("Failed = %+v, want empty", got.Failed)
				}

				after, err := db.GetDesiredService(ctx, "web")
				if err != nil {
					t.Fatalf("GetDesiredService(web) error = %v", err)
				}
				if after.RestartNonce == webNonceBefore {
					t.Error("RestartNonce unchanged after restart")
				}

				unrelated, err := db.GetDesiredService(ctx, "unrelated")
				if err != nil {
					t.Fatalf("GetDesiredService(unrelated) error = %v", err)
				}
				if unrelated.RestartNonce != "" {
					t.Error("RestartNonce changed for an app outside the project")
				}
			},
		},
		{
			name:      "no apps",
			projectID: "proj_1",
			setup: func(t *testing.T, ctx context.Context, db *store.DB) {
				if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "empty"}); err != nil {
					t.Fatalf("SaveProject() error = %v", err)
				}
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, _ context.Context, _ *store.DB, got projectRestartResponse) {
				if got.RestartedCount != 0 || len(got.Apps) != 0 || len(got.Failed) != 0 {
					t.Errorf("response = %+v, want an all-empty result", got)
				}
			},
		},
		{
			name:       "not found",
			projectID:  "does-not-exist",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			ctx := context.Background()

			if tt.setup != nil {
				tt.setup(t, ctx, db)
			}

			rec := httptest.NewRecorder()
			target := "/api/v1/projects/" + tt.projectID + "/restart"
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, target, ""))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.check == nil {
				return
			}
			var got projectRestartResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			tt.check(t, ctx, db, got)
		})
	}
}
