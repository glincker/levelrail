package api

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/attention"
	"github.com/glincker/levelrail/internal/store"
)

func seedAnnouncement(t *testing.T, db *store.DB, rows ...store.AnnouncementRow) {
	t.Helper()
	now := time.Now().UTC()
	st := store.AnnouncementFeedState{
		LastSequence: 1, VerifiedAt: store.FormatAuditTime(now), FetchedAt: store.FormatAuditTime(now),
		ExpiresAt: store.FormatAuditTime(now.Add(24 * time.Hour)),
	}
	if err := db.SaveAnnouncementFeed(context.Background(), st, rows, nil); err != nil {
		t.Fatal(err)
	}
}

func announcementRow(id, tier, appliesTo string) store.AnnouncementRow {
	now := time.Now().UTC()
	payload, _ := json.Marshal(map[string]any{
		"id": id, "tier": tier, "kind": "announcement", "title": "Title " + id, "body": "Body " + id,
		"applies_to": appliesTo, "issued_at": now.Add(-time.Hour), "expires_at": now.Add(24 * time.Hour),
	})
	return store.AnnouncementRow{
		ID: id, Tier: tier, Kind: "announcement", PayloadJSON: string(payload),
		IssuedAt: store.FormatAuditTime(now.Add(-time.Hour)), ExpiresAt: store.FormatAuditTime(now.Add(24 * time.Hour)), Sequence: 1,
	}
}

func listAnnouncements(t *testing.T, rt *Router, req *http.Request) announcementsResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out announcementsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func announcementIDs(items []announcementView) map[string]announcementView {
	out := map[string]announcementView{}
	for _, v := range items {
		out[v.ID] = v
	}
	return out
}

func newAnnouncementRouter(t *testing.T) (*Router, *http.Cookie, *store.DB) {
	t.Helper()
	setVersion(t, "v1.5.13")
	t.Setenv("APP_ANNOUNCEMENTS", "all")
	rt, db := newTestRouter(t)
	seedAnnouncement(t, db,
		announcementRow("crit", "critical", "<1.5.14"),
		announcementRow("adv", "advisory", "<1.5.14"),
		announcementRow("feat", "feature", ""),
		announcementRow("future", "advisory", ">=2.0.0"),
	)
	return rt, loginTestSession(t, rt, db), db
}

func TestAnnouncementsAbilityAndVersionFiltering(t *testing.T) {
	rt, cookie, db := newAnnouncementRouter(t)
	got := announcementIDs(listAnnouncements(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/announcements", "")).Items)
	if len(got) != 3 || got["crit"].ID == "" || got["adv"].ID == "" || got["feat"].ID == "" {
		t.Fatalf("root sees security and feature items matching this version: %v", got)
	}
	if got["crit"].Dismissible || !got["adv"].Dismissible {
		t.Fatalf("critical is not dismissible: %+v", got)
	}
	tok := seedMatrixToken(t, db, "reader", []string{AbilityRead})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/announcements", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	got = announcementIDs(listAnnouncements(t, rt, req).Items)
	if len(got) != 1 || got["feat"].ID == "" {
		t.Fatalf("read-only caller must not see security tiers: %v", got)
	}
}

func TestAnnouncementsOffShowsNothing(t *testing.T) {
	rt, cookie, _ := newAnnouncementRouter(t)
	t.Setenv("APP_ANNOUNCEMENTS", "off")
	out := listAnnouncements(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/announcements", ""))
	if len(out.Items) != 0 || out.Items == nil {
		t.Fatalf("off must return an empty list: %+v", out.Items)
	}
}

func TestAnnouncementCriticalCannotBeDismissedButSnoozes(t *testing.T) {
	rt, cookie, _ := newAnnouncementRouter(t)
	post := func(path, body string) int {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, path, body))
		return rec.Code
	}
	if code := post("/api/v1/announcements/crit/dismiss", ""); code != http.StatusConflict {
		t.Fatalf("critical dismiss = %d, want 409", code)
	}
	banner := func() int {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/announcements/banner", ""))
		var out map[string][]announcementView
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return len(out["items"])
	}
	if banner() != 1 {
		t.Fatal("critical item must be in the banner")
	}
	if code := post("/api/v1/announcements/crit/snooze", `{"duration":"9999h"}`); code != http.StatusNoContent {
		t.Fatalf("snooze = %d", code)
	}
	if banner() != 0 {
		t.Fatal("snoozed critical leaves the banner")
	}
	out := listAnnouncements(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/announcements", ""))
	until := announcementIDs(out.Items)["crit"].SnoozedUntil
	if until == nil || time.Until(*until) > 25*time.Hour {
		t.Fatalf("snooze must be capped at the 24h default, got %v", until)
	}
	if code := post("/api/v1/announcements/feat/dismiss", ""); code != http.StatusNoContent {
		t.Fatalf("dismiss = %d", code)
	}
	if code := post("/api/v1/announcements/nope/dismiss", ""); code != http.StatusNotFound {
		t.Fatalf("unknown id = %d", code)
	}
	out = listAnnouncements(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/announcements", ""))
	if !announcementIDs(out.Items)["feat"].Dismissed {
		t.Fatal("dismissed flag not set")
	}
}

func TestAnnouncementAttentionSource(t *testing.T) {
	rt, cookie, _ := newAnnouncementRouter(t)
	ids := map[string]bool{}
	for _, it := range fetchFeed(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/attention/feed", "")) {
		ids[it.ID] = true
		if it.Kind == attentionKindAnnouncement && it.Class != attention.ClassRecommendation {
			t.Errorf("class = %q", it.Class)
		}
	}
	if !ids["announcement:adv"] || !ids["announcement:feat"] || ids["announcement:crit"] {
		t.Fatalf("attention shows non-critical notices only: %v", ids)
	}
}

func TestAnnouncementDevKeysIgnoredInProductionMode(t *testing.T) {
	rt, cookie, _ := newAnnouncementRouter(t)
	pub, _, _ := ed25519.GenerateKey(nil)
	t.Setenv("APP_ANNOUNCEMENTS_DEV_KEYS", "D:"+base64.StdEncoding.EncodeToString(pub)+":info")
	status := func() bool {
		return listAnnouncements(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/announcements", "")).Status.TrustConfigured
	}
	t.Setenv("APP_DEV_MODE", "")
	if status() {
		t.Fatal("dev keys must be refused outside dev mode")
	}
	t.Setenv("APP_DEV_MODE", "1")
	if !status() {
		t.Skip("release build ignores APP_DEV_MODE entirely")
	}
}
