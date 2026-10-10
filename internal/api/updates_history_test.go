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

func TestUpgradeHistoryListAndAck(t *testing.T) {
	setVersion(t, "v0.0.2")
	rt, db := newTestRouter(t)
	rt.brand.RepoURL = "https://github.com/acme/widget"
	rt.upgradeHistory = db
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	e, ok, err := db.InsertUpgradeHistory(ctx, store.UpgradeHistoryEntry{
		Kind: "upgraded", FromVersion: "v0.0.1", ToVersion: "v0.0.2", SchemaBefore: 10, SchemaAfter: 12,
		OccurredAt: time.Now(), Initiator: "unknown", Health: "booted",
	})
	if err != nil || !ok {
		t.Fatalf("insert: %v %v", err, ok)
	}

	get := func() upgradeHistoryResource {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/updates/history", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		var out upgradeHistoryResource
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	got := get()
	if len(got.Entries) != 1 || got.Unacknowledged != 1 {
		t.Fatalf("history = %+v", got)
	}
	it := got.Entries[0]
	if !it.SchemaMoved || it.ReleaseURL != "https://github.com/acme/widget/releases/tag/v0.0.2" ||
		it.CompareURL != "https://github.com/acme/widget/compare/v0.0.1...v0.0.2" {
		t.Fatalf("item = %+v", it)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/updates/history/"+e.ID+"/ack", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("ack status %d: %s", rec.Code, rec.Body.String())
	}
	after := get()
	if after.Unacknowledged != 0 || !after.Entries[0].Acknowledged || after.Entries[0].AckedBy == "" {
		t.Fatalf("after ack = %+v", after.Entries[0])
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/updates/history/uh_nope/ack", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing id status %d", rec.Code)
	}
}
