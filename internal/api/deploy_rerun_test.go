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

func TestTriggerDeployRerunOfValidation(t *testing.T) {
	tests := []struct {
		name       string
		rerunOf    string
		wantStatus int
		wantStored string
		wantMsg    string
	}{
		{name: "unknown id", rerunOf: "dep_missing", wantStatus: http.StatusBadRequest, wantMsg: "no such deploy attempt"},
		{name: "other app", rerunOf: "dep_other", wantStatus: http.StatusBadRequest, wantMsg: "no such deploy attempt"},
		{name: "valid", rerunOf: "dep_src", wantStatus: http.StatusAccepted, wantStored: "dep_src"},
		{name: "not a rerun", rerunOf: "", wantStatus: http.StatusAccepted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			ctx := context.Background()
			for _, n := range []string{"web", "api"} {
				if err := db.SaveDesiredService(ctx, store.DesiredService{Name: n, Image: "levelrail/" + n + ":1", Port: 3000}); err != nil {
					t.Fatalf("seed app: %v", err)
				}
			}
			base := time.Now().UTC().Add(-time.Hour)
			for id, app := range map[string]string{"dep_src": "web", "dep_other": "api"} {
				if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
					ID: id, ServiceName: app, Image: "levelrail/" + app + ":1",
					Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusSucceeded, StartedAt: base,
				}); err != nil {
					t.Fatalf("seed attempt: %v", err)
				}
			}

			body := `{"image":"levelrail/web:2"}`
			if tc.rerunOf != "" {
				body = `{"image":"levelrail/web:2","rerun_of":"` + tc.rerunOf + `"}`
			}
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", body))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantMsg != "" && !strings.Contains(rec.Body.String(), tc.wantMsg) {
				t.Errorf("body = %s, want it to contain %q", rec.Body.String(), tc.wantMsg)
			}
			if tc.wantStatus != http.StatusAccepted {
				return
			}
			attempts, err := db.ListDeployAttempts(ctx, "web")
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			var created *store.DeployAttempt
			for i := range attempts {
				if attempts[i].ID != "dep_src" {
					created = &attempts[i]
				}
			}
			if created == nil {
				t.Fatalf("no new attempt recorded: %+v", attempts)
			}
			if created.RerunOf != tc.wantStored {
				t.Errorf("RerunOf = %q, want %q", created.RerunOf, tc.wantStored)
			}
		})
	}
}

func TestRerunChainPositions(t *testing.T) {
	tests := []struct {
		name     string
		attempts []store.DeployAttempt
		want     map[string]int
	}{
		{name: "single", attempts: []store.DeployAttempt{{ID: "a"}}, want: map[string]int{"a": 1}},
		{
			name:     "chain of three",
			attempts: []store.DeployAttempt{{ID: "c", RerunOf: "b"}, {ID: "b", RerunOf: "a"}, {ID: "a"}},
			want:     map[string]int{"a": 1, "b": 2, "c": 3},
		},
		{
			name:     "cycle terminates",
			attempts: []store.DeployAttempt{{ID: "a", RerunOf: "b"}, {ID: "b", RerunOf: "a"}},
			want:     map[string]int{"a": 2, "b": 2},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pos := rerunChainPositions(tc.attempts)
			for id, want := range tc.want {
				if got := pos(id); got != want {
					t.Errorf("position(%s) = %d, want %d", id, got, want)
				}
			}
		})
	}
}

func TestRerunChainPositionsHonoursMaxDepth(t *testing.T) {
	t.Setenv(envRerunChainMaxDepth, "2")
	pos := rerunChainPositions([]store.DeployAttempt{{ID: "c", RerunOf: "b"}, {ID: "b", RerunOf: "a"}, {ID: "a"}})
	if got := pos("c"); got != 2 {
		t.Errorf("position(c) = %d, want 2 (capped)", got)
	}
}

func TestDeployAttemptResourceExposesRerunFields(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	base := time.Now().UTC().Add(-time.Hour)
	for i, a := range []store.DeployAttempt{{ID: "dep_1"}, {ID: "dep_2", RerunOf: "dep_1"}, {ID: "dep_3", RerunOf: "dep_2"}} {
		a.ServiceName, a.Image, a.Source = "web", "levelrail/web:1", store.DeployAttemptSourceImage
		a.Status, a.StartedAt = store.DeployAttemptStatusFailed, base.Add(time.Duration(i)*time.Minute)
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatalf("seed attempt: %v", err)
		}
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploy-attempts", ""))
	var list []deployAttemptResource
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 3 || list[0].ID != "dep_3" || list[0].RerunOf != "dep_2" || list[0].ChainPosition != 3 || list[2].ChainPosition != 1 {
		t.Errorf("list = %+v", list)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_2", ""))
	var one deployAttemptResource
	if err := json.Unmarshal(rec.Body.Bytes(), &one); err != nil {
		t.Fatalf("decode one: %v", err)
	}
	if one.RerunOf != "dep_1" || one.ChainPosition != 2 {
		t.Errorf("single = rerun_of %q position %d, want dep_1 and 2", one.RerunOf, one.ChainPosition)
	}
}
