package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestBulkNeedsCountConfirm(t *testing.T) {
	tests := []struct {
		action string
		n      int
		want   bool
	}{
		{bulkActionRestart, 10, false},
		{bulkActionRestart, 11, true},
		{bulkActionStop, 11, true},
		{bulkActionRedeploy, 50, true},
		{bulkActionAddTag, 50, false},
		{bulkActionDelete, 50, false},
	}
	for _, tc := range tests {
		if got := bulkNeedsCountConfirm(tc.action, tc.n); got != tc.want {
			t.Errorf("bulkNeedsCountConfirm(%s, %d) = %v, want %v", tc.action, tc.n, got, tc.want)
		}
	}
}

func TestBulkRestartAboveThresholdNeedsConfirmCount(t *testing.T) {
	t.Setenv("APP_BULK_CONFIRM_ABOVE", "1")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "a-app")
	seedScheduledTaskApp(t, db, "b-app")
	body := `{"action":"restart","names":["a-app","b-app"]`

	if code, _ := bulkCall(t, rt, cookie, body+`}`); code != http.StatusConflict {
		t.Fatalf("no confirm_count status = %d, want 409", code)
	}
	if code, resp := bulkCall(t, rt, cookie, body+`,"dry_run":true}`); code != http.StatusMultiStatus || !resp.DryRun {
		t.Fatalf("dry run status = %d, want 207", code)
	}
	if code, _ := bulkCall(t, rt, cookie, body+`,"confirm_count":2}`); code != http.StatusMultiStatus {
		t.Fatalf("confirmed status = %d, want 207", code)
	}
}

func TestDrainNodeDryRunChangesNothing(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seedNode(t, db, "node_1", "worker-1")
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateServiceNode(ctx, "web", "node_1"); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/node_1/drain?dry_run=true", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var got drainPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.DryRun || len(got.Services) != 1 || got.Services[0] != "web" {
		t.Fatalf("preview = %+v", got)
	}
	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil || svc.NodeID != "node_1" {
		t.Fatalf("service moved by a dry run: %+v, %v", svc, err)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/missing/drain?dry_run=true", ""))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "not found") {
		t.Fatalf("missing node = %d %s", rec.Code, fmt.Sprint(rec.Body.String()))
	}
}
