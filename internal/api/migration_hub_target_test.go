package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

type hubOwnerCase struct {
	t      *testing.T
	rt     *Router
	db     *store.DB
	owner  *http.Cookie
	other  *http.Cookie
	admin  *http.Cookie
	sessID string
}

func (f *hubOwnerCase) call(c *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	return serve(f.rt, authedRequest(f.t, c, method, "/api/v1/migration/hub/sessions"+path, body))
}

func newHubOwnerCase(t *testing.T) *hubOwnerCase {
	t.Helper()
	rt, db, admin := newSafetyRouter(t, stubResolver{digest: "sha256:abc"})
	rt.execRuntime = func(string) (docker.Runtime, error) { return &hubRuntime{}, nil }
	abil := []string{AbilityRead, AbilityWrite, AbilityWriteSensitive, AbilityReadSensitive}
	for _, id := range []string{"user_hub_a", "user_hub_b"} {
		u := store.User{ID: id, Email: id + "@example.com", DisplayName: id, Abilities: abil, CreatedAt: time.Now()}
		if err := db.CreateUser(context.Background(), u); err != nil {
			t.Fatal(err)
		}
	}
	f := &hubOwnerCase{t: t, rt: rt, db: db, admin: admin, owner: sessionCookieForTest(t, rt, "user_hub_a"), other: sessionCookieForTest(t, rt, "user_hub_b")}
	rec := f.call(f.owner, http.MethodPost, "", `{"engine":"postgres","host":"old.example.com","user":"u","password":"`+hubTestPassword+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body)
	}
	var sess hubSessionResource
	_ = json.Unmarshal(rec.Body.Bytes(), &sess)
	f.sessID = sess.ID
	return f
}

func (f *hubOwnerCase) applyAndWait() {
	f.t.Helper()
	if rec := f.call(f.owner, http.MethodPost, "/"+f.sessID+"/apply", `{}`); rec.Code != http.StatusAccepted {
		f.t.Fatalf("apply = %d: %s", rec.Code, rec.Body)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var v hubSessionResource
		_ = json.Unmarshal(f.call(f.owner, http.MethodGet, "/"+f.sessID, "").Body.Bytes(), &v)
		if !v.Running && v.Summary.Verified+v.Summary.Failed == 3 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	f.t.Fatal("copy did not finish")
}

func TestHubSelectionRenameBeforeApply(t *testing.T) {
	f := newHubOwnerCase(t)
	if err := f.db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "orders", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatal(err)
	}
	put := func(name string) int {
		return f.call(f.owner, http.MethodPut, "/"+f.sessID+"/selection", `{"items":[{"source_db":"a","target_name":"`+name+`"}]}`).Code
	}
	for _, bad := range []string{"orders", "web", "Bad Name!"} {
		if code := put(bad); code != http.StatusConflict && code != http.StatusBadRequest {
			t.Errorf("rename to %q = %d, want refusal", bad, code)
		}
	}
	if code := put("fresh-db"); code != http.StatusOK {
		t.Fatalf("rename to a free name = %d, want 200", code)
	}
}

func TestHubSelectionRefusedAfterVerifyAndConnectionStaysOnOriginal(t *testing.T) {
	f := newHubOwnerCase(t)
	f.applyAndWait()
	if err := f.db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "orders", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatal(err)
	}
	rec := f.call(f.owner, http.MethodPut, "/"+f.sessID+"/selection", `{"items":[{"source_db":"a","target_name":"orders"}]}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("rename after verify = %d, want 409", rec.Code)
	}
	var conn hubConnection
	rec = f.call(f.owner, http.MethodGet, "/"+f.sessID+"/items/a/connection", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &conn)
	if conn.Database != "a" {
		t.Fatalf("connection database = %q, want a", conn.Database)
	}
	if rec := f.call(f.owner, http.MethodPut, "/"+f.sessID+"/selection", `{"items":[{"source_db":"a","target_name":"a"}]}`); rec.Code != http.StatusOK {
		t.Errorf("unchanged name = %d, want 200", rec.Code)
	}
}

func TestHubApplyRefusesSwappedTarget(t *testing.T) {
	f := newHubOwnerCase(t)
	f.applyAndWait()
	ctx := context.Background()
	if err := f.db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "orders", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatal(err)
	}
	// A tampered row: the plan now points at another team's database.
	if err := f.db.SetMigrationItemPlan(ctx, f.sessID, "a", "orders", "16", true); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetMigrationItemStatus(ctx, f.sessID, "a", store.HubItemFailed, "x", 0, 0, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if rec := f.call(f.owner, http.MethodPost, "/"+f.sessID+"/apply", `{}`); rec.Code == http.StatusAccepted {
		t.Fatalf("apply onto a swapped target = %d, want refusal", rec.Code)
	}
	if rec := f.call(f.owner, http.MethodPost, "/"+f.sessID+"/items/a/reveal", ``); rec.Code != http.StatusNotFound {
		t.Errorf("reveal of a foreign target = %d, want 404", rec.Code)
	}
}

func TestHubSessionOwnerScoping(t *testing.T) {
	f := newHubOwnerCase(t)
	base := "/" + f.sessID
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPut, base + "/selection", `{"items":[{"source_db":"a","selected":false}]}`},
		{http.MethodPut, base + "/step", `{"step":"preflight"}`},
		{http.MethodPost, base + "/apply", `{}`},
		{http.MethodPost, base + "/items/a/reveal", ``},
		{http.MethodDelete, base, ``},
	} {
		if rec := f.call(f.other, c.method, c.path, c.body); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s by another user = %d, want 404", c.method, c.path, rec.Code)
		}
	}
	if rec := f.call(f.admin, http.MethodPut, base+"/selection", `{"items":[{"source_db":"a","selected":false}]}`); rec.Code != http.StatusOK {
		t.Errorf("root mutation = %d, want 200", rec.Code)
	}
}
