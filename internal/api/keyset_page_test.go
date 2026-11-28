package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

func getPage(t *testing.T, rt *Router, cookie *http.Cookie, path string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, path, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s", path, rec.Code, rec.Body.String())
	}
	return rec, rec.Header().Get(nextCursorHeader)
}

func TestKeysetPaging_DeployAttemptsAndNodeEvents(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Truncate(time.Millisecond)
	for i := 1; i <= 5; i++ {
		a := store.DeployAttempt{ID: fmt.Sprintf("dep_%d", i), ServiceName: "web", Image: "levelrail/web:1", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusSucceeded, StartedAt: base.Add(time.Duration(i) * time.Minute)}
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatal(err)
		}
	}

	// A row inserted between pages must not shift the next page.
	var ids []string
	path, injected := "/api/v1/apps/web/deploy-attempts?limit=2", false
	for {
		rec, next := getPage(t, rt, cookie, path)
		var page []deployAttemptResource
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		for _, a := range page {
			ids = append(ids, a.ID)
		}
		if !injected {
			injected = true
			if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: "dep_new", ServiceName: "web", Image: "levelrail/web:1", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusSucceeded, StartedAt: base.Add(time.Hour)}); err != nil {
				t.Fatal(err)
			}
		}
		if next == "" {
			break
		}
		path = "/api/v1/apps/web/deploy-attempts?limit=2&cursor=" + next
	}
	if fmt.Sprint(ids) != "[dep_5 dep_4 dep_3 dep_2 dep_1]" {
		t.Fatalf("paged ids = %v", ids)
	}
	bad := httptest.NewRecorder()
	rt.Handler().ServeHTTP(bad, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploy-attempts?cursor=bogus", ""))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad cursor = %d, want 400", bad.Code)
	}

	seedNode(t, db, "node_a", "alpha")
	for _, s := range []store.NodeStatus{store.NodeStatusOnline, store.NodeStatusOffline, store.NodeStatusOnline} {
		if err := db.UpdateNodeStatus(ctx, "node_a", s); err != nil {
			t.Fatal(err)
		}
	}
	rec, next := getPage(t, rt, cookie, "/api/v1/nodes/node_a/events?limit=2")
	var first, second []nodeStatusEventResource
	_ = json.Unmarshal(rec.Body.Bytes(), &first)
	if len(first) != 2 || next == "" {
		t.Fatalf("first page = %d events, next %q", len(first), next)
	}
	rec, next = getPage(t, rt, cookie, "/api/v1/nodes/node_a/events?limit=2&cursor="+next)
	_ = json.Unmarshal(rec.Body.Bytes(), &second)
	if len(second) != 1 || next != "" {
		t.Fatalf("second page = %d events, next %q", len(second), next)
	}
}

func TestKeysetPaging_AppsList(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	for _, n := range []string{"a1", "a2", "a3", "a4", "a5"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: n, Image: "levelrail/web:1", Port: 3000}); err != nil {
			t.Fatal(err)
		}
	}
	// An app added behind the cursor must not shift or repeat later pages.
	var names []string
	path, injected := "/api/v1/apps?limit=2", false
	for {
		rec, next := getPage(t, rt, cookie, path)
		var page []appListResource
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		for _, a := range page {
			names = append(names, a.Name)
		}
		if !injected {
			injected = true
			if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "a0", Image: "levelrail/web:1", Port: 3000}); err != nil {
				t.Fatal(err)
			}
		}
		if next == "" {
			break
		}
		path = "/api/v1/apps?limit=2&cursor=" + next
	}
	if fmt.Sprint(names) != "[a1 a2 a3 a4 a5]" {
		t.Fatalf("paged names = %v", names)
	}
	bad := httptest.NewRecorder()
	rt.Handler().ServeHTTP(bad, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps?cursor=bogus", ""))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad cursor = %d, want 400", bad.Code)
	}
}
