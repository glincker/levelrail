package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/functions"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDeletedFunctionSecretCannotTriggerSameNameApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.gitSourceSecrets = newFakeGitSourceSecrets()
	WithFunctionTriggers(db, functions.TriggerConfigFromEnv(func(string) (string, bool) { return "", false }), nil)(rt)
	ctx := context.Background()
	do := func(req *http.Request) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec
	}
	create := func() {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "fn", Image: "nginx:1", Port: 80}); err != nil {
			t.Fatal(err)
		}
	}
	call := func(secret string) *httptest.ResponseRecorder {
		body := []byte(`{"x":1}`)
		ts := time.Now()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/functions/fn", bytes.NewReader(body))
		req.Header.Set(functions.HeaderTimestamp, strconv.FormatInt(ts.Unix(), 10))
		req.Header.Set(functions.HeaderSignature, functions.Sign(secret, ts.Unix(), body))
		return do(req)
	}
	enable := func() string {
		rec := do(authedRequest(t, cookie, http.MethodPut, "/api/v1/functions/fn/triggers/webhook", `{"path":"/hook"}`))
		var v functionWebhookView
		if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil || v.Secret == "" {
			t.Fatalf("enable = %d %s", rec.Code, rec.Body)
		}
		return v.Secret
	}

	create()
	old := enable()
	if rec := call(old); rec.Code != http.StatusAccepted {
		t.Fatalf("live webhook = %d", rec.Code)
	}
	if _, err := rt.deleteApp(ctx, "fn"); err != nil {
		t.Fatalf("delete = %v", err)
	}
	create()
	if _, err := db.GetFunctionWebhook(ctx, "fn"); err == nil {
		t.Fatal("webhook row survived the app delete")
	}
	want := do(httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/functions/never-existed", nil))
	got := call(old)
	errText := func(rec *httptest.ResponseRecorder) string {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		return e.Error
	}
	if got.Code != http.StatusNotFound || got.Code != want.Code || errText(got) != errText(want) {
		t.Fatalf("old secret = %d %q, unknown webhook = %d %q", got.Code, got.Body, want.Code, want.Body)
	}

	// A row that outlived its app (written before the cleanup existed) is rejected by its binding.
	new1 := enable()
	if rec := call(new1); rec.Code != http.StatusAccepted {
		t.Fatalf("new life webhook = %d", rec.Code)
	}
	if err := db.DeleteDesiredService(ctx, "fn"); err != nil {
		t.Fatal(err)
	}
	create()
	if rec := call(new1); rec.Code != http.StatusNotFound {
		t.Fatalf("secret of an earlier life = %d, want 404", rec.Code)
	}
}

func TestDeletedAppKeyValueNotReadableBySameNameApp(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "kvfn", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	lim := store.KVLimits{MaxKeyBytes: 64, MaxKeys: 10, MaxValueBytes: 1024, MaxTotalBytes: 4096}
	if err := db.PutFunctionKV(ctx, "kvfn", "k", []byte("secret-value"), 0, lim, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.deleteApp(ctx, "kvfn"); err != nil {
		t.Fatalf("delete = %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "kvfn", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetFunctionKV(ctx, "kvfn", "k", time.Now()); err == nil {
		t.Fatal("a same-name app read the deleted app's key-value data")
	}
}

func TestSoftDeleteKeepsFunctionDataUntilFinalPurge(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "softfn", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	lim := store.KVLimits{MaxKeyBytes: 64, MaxKeys: 10, MaxValueBytes: 1024, MaxTotalBytes: 4096}
	if err := db.PutFunctionKV(ctx, "softfn", "k", []byte("v"), 0, lim, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.SetFunctionWebhook(ctx, "softfn", "/", "", false, time.Now()); err != nil {
		t.Fatal(err)
	}
	kept := withSoftDelete(store.WithKeptRevisions(ctx), func(c context.Context) error {
		return db.AddRecentlyDeleted(c, store.RecentlyDeleted{Kind: store.RecentlyDeletedApp, Name: "softfn", Snapshot: "{}"})
	})
	if _, err := rt.deleteApp(kept, "softfn"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetFunctionKV(ctx, "softfn", "k", time.Now()); err != nil {
		t.Fatalf("kv lost inside the undo window: %v", err)
	}
	if _, err := db.GetFunctionWebhook(ctx, "softfn"); err != nil {
		t.Fatalf("webhook lost inside the undo window: %v", err)
	}
	if _, err := db.PurgeRecentlyDeleted(ctx, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetFunctionKV(ctx, "softfn", "k", time.Now()); err == nil {
		t.Fatal("kv survived the final purge")
	}
	if _, err := db.GetFunctionWebhook(ctx, "softfn"); err == nil {
		t.Fatal("webhook survived the final purge")
	}
}

func TestLegacyUnboundWebhookNeverReachesNewApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	secrets := newFakeGitSourceSecrets()
	rt.gitSourceSecrets = secrets
	WithFunctionTriggers(db, functions.TriggerConfigFromEnv(func(string) (string, bool) { return "", false }), nil)(rt)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "legacy", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	// A row and secret left by an earlier app, written before webhooks were bound to an app life.
	if err := db.SetFunctionWebhook(ctx, "legacy", "/", "", false, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := secrets.SetValue(ctx, functionWebhookSecretNS+"legacy", functionWebhookSecretKey, "old-secret"); err != nil {
		t.Fatal(err)
	}
	do := func(req *http.Request) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec
	}
	call := func(secret string) int {
		body := []byte(`{}`)
		ts := time.Now()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/functions/legacy", bytes.NewReader(body))
		req.Header.Set(functions.HeaderTimestamp, strconv.FormatInt(ts.Unix(), 10))
		req.Header.Set(functions.HeaderSignature, functions.Sign(secret, ts.Unix(), body))
		return do(req).Code
	}
	if got := call("old-secret"); got != http.StatusNotFound {
		t.Fatalf("old secret = %d, want 404", got)
	}
	rec := do(authedRequest(t, cookie, http.MethodPut, "/api/v1/functions/legacy/triggers/webhook", ""))
	var v functionWebhookView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil || v.Secret == "" || v.Secret == "old-secret" {
		t.Fatalf("enable must mint a fresh secret: %d %s", rec.Code, rec.Body)
	}
	if got := call("old-secret"); got != http.StatusNotFound && got != http.StatusUnauthorized {
		t.Fatalf("old secret after enable = %d", got)
	}
	if got := call(v.Secret); got != http.StatusAccepted {
		t.Fatalf("fresh secret = %d, want 202", got)
	}
}

type failingPurgeStore struct{ *store.DB }

func (failingPurgeStore) PurgeRecentlyDeleted(context.Context, time.Time) (int, error) {
	return 0, errors.New("injected purge failure")
}

func TestFailedUndoPurgeKeepsNameReserved(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	if err := db.AddRecentlyDeleted(ctx, store.RecentlyDeleted{Kind: store.RecentlyDeletedApp, Name: "held", Snapshot: "{}"}); err != nil {
		t.Fatal(err)
	}
	// Age the row past the window so the lookup tries to purge it.
	if _, err := db.ExecContext(ctx, `UPDATE recently_deleted SET deleted_at = '2000-01-01T00:00:00Z' WHERE name = 'held'`); err != nil {
		t.Fatal(err)
	}
	rt.apps = failingPurgeStore{db}
	if !rt.deletedNameReserved(ctx, "held") {
		t.Fatal("name was freed although the purge failed")
	}
}
