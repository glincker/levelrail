package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/deployurls"
	"github.com/glincker/levelrail/internal/store"
)

func TestRetainedCopyDoesNotReturnAfterDelete(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "web:bbb", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	for i, sha := range []string{"aaa", "bbb"} {
		a := store.DeployAttempt{ID: "da_" + sha, ServiceName: "web", Image: "web:" + sha, CommitSHA: sha, Status: store.DeployAttemptStatusSucceeded,
			StartedAt: time.Now().Add(time.Duration(i) * time.Minute), ImageDigest: "sha256:" + sha, DigestReason: store.DigestReasonResolved}
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	s := &deployurls.Syncer{
		Store: db, Config: deployurls.Config{Keep: 3, IdleMinutes: 5, Interval: time.Minute, MaxCopies: 2, Mode: deployurls.ModePublic},
		HostFor: func(_ context.Context, n string) string { return n + ".apps.example.com" },
		Remove:  rt.RemoveDeployURLService, HostMemory: func() (int64, int64, error) { return 8 << 30, 6 << 30, nil },
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListDeployURLs(ctx, "web")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %v", rows)
	}
	sib := rows[0].ServiceName

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetDesiredService(ctx, sib); !errors.Is(err, store.ErrServiceNotFound) {
		t.Fatalf("copy %s still exists after delete and sync: %v", sib, err)
	}
	if left, err := rt.listPublicApps(ctx); err != nil || len(left) != 0 {
		t.Fatalf("apps after delete = %v, err = %v", left, err)
	}
}
