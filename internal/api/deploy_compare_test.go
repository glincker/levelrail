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

func TestHandleCompareDeploys_TwoAttempts(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:2", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	base := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_1", ServiceName: "web", Image: "levelrail/web:1",
		CommitSHA: "sha1", Source: store.DeployAttemptSourceWebhook,
		Status: store.DeployAttemptStatusSucceeded, StartedAt: base,
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_2", ServiceName: "web", Image: "levelrail/web:2",
		CommitSHA: "sha2", Source: store.DeployAttemptSourceManual,
		Status: store.DeployAttemptStatusSucceeded, StartedAt: base.Add(time.Minute),
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/compare?from=dep_1&to=dep_2", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got deployCompareResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.From.DeployID != "dep_1" || got.To.DeployID != "dep_2" {
		t.Errorf("from/to ids = %q/%q, want dep_1/dep_2", got.From.DeployID, got.To.DeployID)
	}
	if got.To.IsCurrent {
		t.Error("to.IsCurrent = true, want false for two explicit attempts")
	}

	wantChanges := map[string][2]string{
		"image":      {"levelrail/web:1", "levelrail/web:2"},
		"commit_sha": {"sha1", "sha2"},
		"source":     {store.DeployAttemptSourceWebhook, store.DeployAttemptSourceManual},
	}
	if len(got.Changes) != len(wantChanges) {
		t.Fatalf("got %d changes, want %d: %+v", len(got.Changes), len(wantChanges), got.Changes)
	}
	for _, c := range got.Changes {
		want, ok := wantChanges[c.Field]
		if !ok {
			t.Errorf("unexpected changed field %q", c.Field)
			continue
		}
		if c.From != want[0] || c.To != want[1] {
			t.Errorf("field %q = %q -> %q, want %q -> %q", c.Field, c.From, c.To, want[0], want[1])
		}
	}
	if len(got.UnsnapshottedFields) == 0 {
		t.Error("UnsnapshottedFields is empty, want the non-captured field list")
	}
	if got.Note == "" {
		t.Error("Note is empty, want an explicit limitation message")
	}
}

func TestHandleCompareDeploys_AgainstCurrent(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:current", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_1", ServiceName: "web", Image: "levelrail/web:1",
		Source: store.DeployAttemptSourceImage,
		Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/compare?from=dep_1", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got deployCompareResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.To.IsCurrent {
		t.Error("to.IsCurrent = false, want true when ?to is omitted")
	}
	if got.To.DeployID != "" {
		t.Errorf("to.DeployID = %q, want empty for the current side", got.To.DeployID)
	}
	if got.To.Image != "levelrail/web:current" {
		t.Errorf("to.Image = %q, want the app's current desired image", got.To.Image)
	}
	if got.To.Source != "" || got.To.Status != "" {
		t.Errorf("to.Source/Status = %q/%q, want empty for the current side", got.To.Source, got.To.Status)
	}

	foundImageChange := false
	for _, c := range got.Changes {
		if c.Field == "source" {
			t.Error("source should never be reported as changed against the current side")
		}
		if c.Field == "image" {
			foundImageChange = true
		}
	}
	if !foundImageChange {
		t.Error("expected an image change between dep_1 and the current image")
	}
}

func TestHandleCompareDeploys_MissingFrom(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/compare", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleCompareDeploys_AttemptNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/compare?from=dep_ghost", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleCompareDeploys_AttemptFromDifferentApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "other", Image: "levelrail/other:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_other", ServiceName: "other", Image: "levelrail/other:1",
		Source: store.DeployAttemptSourceImage,
		Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/compare?from=dep_other", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleCompareDeploys_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/ghost/deploys/compare?from=dep_1", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
