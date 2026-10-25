package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func seedOfflineNodeApp(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seen := time.Now().Add(-5 * time.Minute)
	if err := db.SaveNode(ctx, store.Node{ID: "node-b", Name: "worker-b", Status: store.NodeStatusOffline, Schedulable: true, LastSeenAt: &seen}); err != nil {
		t.Fatalf("SaveNode: %v", err)
	}
	if err := db.UpdateNodeStatus(ctx, "node-b", store.NodeStatusOffline); err != nil {
		t.Fatalf("UpdateNodeStatus: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.UpdateServiceNode(ctx, "web", "node-b"); err != nil {
		t.Fatalf("UpdateServiceNode: %v", err)
	}
	return rt, db, cookie
}

func TestOfflineNodeRefusesLifecycleRequests(t *testing.T) {
	for _, tc := range []struct{ path, body string }{
		{"/api/v1/apps/web/deploys", `{"image":"levelrail/web:2"}`},
		{"/api/v1/apps/web/restart", ""},
		{"/api/v1/apps/web/stop", ""},
		{"/api/v1/apps/web/start", ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rt, db, cookie := seedOfflineNodeApp(t)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, tc.path, tc.body))
			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
			}
			for _, want := range []string{"node_offline", "worker-b", "apps move", "nodes drain", "5m"} {
				if !strings.Contains(rec.Body.String(), want) {
					t.Errorf("body missing %q: %s", want, rec.Body.String())
				}
			}
			svc, err := db.GetDesiredService(context.Background(), "web")
			if err != nil {
				t.Fatalf("GetDesiredService: %v", err)
			}
			if svc.Image != "levelrail/web:1" || svc.Suspended || svc.RestartNonce != "" {
				t.Errorf("desired state changed: %+v", svc)
			}
			attempts, _ := db.ListDeployAttempts(context.Background(), "web")
			if len(attempts) != 0 {
				t.Errorf("attempts = %d, want 0", len(attempts))
			}
		})
	}
}

func TestOfflineNodeDeployQueueOverrideRecordsPending(t *testing.T) {
	rt, db, cookie := seedOfflineNodeApp(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", `{"image":"levelrail/web:2","queue_while_offline":true}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body = %s", rec.Code, rec.Body.String())
	}
	attempts, err := db.ListDeployAttempts(context.Background(), "web")
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts = %v, err = %v", attempts, err)
	}
	if attempts[0].Status != store.DeployAttemptStatusPending {
		t.Errorf("status = %q, want pending", attempts[0].Status)
	}
	if !strings.Contains(attempts[0].Reason, "worker-b") {
		t.Errorf("reason = %q, want node name", attempts[0].Reason)
	}
}

func TestOnlineNodeDeployUnaffected(t *testing.T) {
	rt, db, cookie := seedOfflineNodeApp(t)
	if err := db.UpdateNodeStatus(context.Background(), "node-b", store.NodeStatusOnline); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/restart", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
