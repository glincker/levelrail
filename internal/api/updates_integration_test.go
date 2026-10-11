package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

func useEdgeChannel(t *testing.T, rt *Router) {
	t.Helper()
	db, ok := rt.updateSettings.(*store.DB)
	if !ok {
		t.Fatal("test router must be backed by *store.DB")
	}
	if err := db.UpdateUpdateSettings(context.Background(), store.UpdateSettings{Channel: upgrade.ChannelEdge}); err != nil {
		t.Fatal(err)
	}
	rt.upgradeFetchers = upgrade.Fetchers{
		Edge: func(context.Context) (*upgrade.Release, error) {
			return &upgrade.Release{
				Tag: "main-07cd3bf", URL: "https://github.com/x/y/commit/07cd3bf", Kind: upgrade.KindEdgeImage,
				Commit: "07cd3bf9a1b2", Image: "ghcr.io/x/y:edge",
			}, nil
		},
	}
	rt.expireReleaseCaches()
}

// An edge target must show guidance and never become a schedule, an
// automatic policy or an Upgrade now, even with scheduling fully configured.
func TestEdgeTargetRefusesScheduleAndAutomaticPolicy(t *testing.T) {
	rt, cookie, launched, _ := newScheduleRouter(t)
	useEdgeChannel(t, rt)
	future := time.Now().Add(6 * time.Hour).UTC().Format(time.RFC3339)

	code, body := suDo(t, rt, cookie, http.MethodGet, "/api/v1/updates/preflight", "")
	if code != http.StatusOK {
		t.Fatalf("preflight %d: %s", code, body)
	}
	pre := decodeBody[updatePreflightResource](t, body)
	if pre.ReleaseKind != upgrade.KindEdgeImage || pre.Edge == nil {
		t.Fatalf("edge guidance missing: %+v", pre)
	}
	for _, c := range pre.Checks {
		if (c.Code == "release_signature" || c.Code == "release_verifier") && c.Status != upgrade.StatusSkipped {
			t.Errorf("%s = %s, want skipped", c.Code, c.Status)
		}
		if c.Status == upgrade.StatusPending {
			t.Errorf("edge must never read as waiting for a release: %+v", c)
		}
	}

	refusals := []struct{ name, method, path, body string }{
		{"schedule latest", http.MethodPost, "/api/v1/updates/schedule", `{"at":"` + future + `"}`},
		{"schedule edge tag", http.MethodPost, "/api/v1/updates/schedule", `{"target":"main-07cd3bf","at":"` + future + `"}`},
		{"upgrade now", http.MethodPost, "/api/v1/updates/self-upgrade", `{"target":"main-07cd3bf"}`},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			code, body := suDo(t, rt, cookie, tc.method, tc.path, tc.body)
			if code < 400 || code >= 500 || !strings.Contains(string(body), "container images") {
				t.Fatalf("status %d, body %s: want a clear 4xx naming container images", code, body)
			}
		})
	}
	code, body = suDo(t, rt, cookie, http.MethodPut, "/api/v1/updates/auto",
		`{"mode":"patch","window_cron":"0 3 * * *","window_duration_minutes":120,"min_release_age_days":0}`)
	if code != http.StatusConflict || !strings.Contains(string(body), "edge") {
		t.Fatalf("automatic policy on edge = %d: %s", code, body)
	}
	if len(*launched) != 0 {
		t.Fatalf("nothing may be launched: %v", *launched)
	}
	_, body = suDo(t, rt, cookie, http.MethodGet, "/api/v1/updates/schedule", "")
	if strings.Contains(string(body), `"target_version":"main-`) {
		t.Fatalf("an edge schedule was stored: %s", body)
	}
}

// A stable release whose checksums are still being uploaded is pending with
// a Re-check fix: it must not be mistaken for the edge skip semantics.
func TestYoungStableReleaseMissingChecksumsIsPendingNotEdgeSkipped(t *testing.T) {
	rt, cookie, _, mgr := newScheduleRouter(t)
	mgr.list = []cpbackup.Info{{Name: "fresh.db", CreatedAt: time.Now()}}
	rt.fetchLatestRelease = func(context.Context) (*githubRelease, error) {
		r := releaseWithAssets(upgrade.ChecksumsAsset)
		r.TagName, r.PublishedAt = "v1.2.0", time.Now().Add(-2*time.Minute).UTC().Format(time.RFC3339)
		return r, nil
	}
	_, body := suDo(t, rt, cookie, http.MethodGet, "/api/v1/updates/preflight?refresh=1", "")
	pre := decodeBody[updatePreflightResource](t, body)
	if pre.ReleaseKind != upgrade.KindRelease || pre.Edge != nil || pre.Channel != upgrade.ChannelStable {
		t.Fatalf("stable release misclassified: %+v", pre)
	}
	var sig upgrade.Check
	for _, c := range pre.Checks {
		if c.Code == "release_signature" {
			sig = c
		}
	}
	if sig.Status != upgrade.StatusPending || sig.Kind != upgrade.KindReleaseIncomplete || !sig.Retryable ||
		sig.Fix == nil || sig.Fix.Action != upgrade.FixRecheck {
		t.Fatalf("want pending release_incomplete with a recheck fix: %+v", sig)
	}
	if !pre.Waiting || !pre.Blocked {
		t.Fatalf("waiting=%v blocked=%v", pre.Waiting, pre.Blocked)
	}

	future := time.Now().Add(6 * time.Hour).UTC().Format(time.RFC3339)
	code, out := suDo(t, rt, cookie, http.MethodPost, "/api/v1/updates/schedule", `{"target":"v1.2.0","at":"`+future+`","ack":["ports","`+headingID()+`"]}`)
	if code != http.StatusCreated {
		t.Fatalf("a pending release can still be scheduled (it is re-checked at fire time): %d %s", code, out)
	}
}

// A dismissed stale failed copy reappears only for a new attempt, and the
// update status the banner reads is independent of the incident feed.
func TestDismissedCopyReturnsForNewAttemptWhileUpdateStatusIsUnaffected(t *testing.T) {
	rt, cookie, _, _ := newScheduleRouter(t)
	db, ok := rt.updateSettings.(*store.DB)
	if !ok {
		t.Fatal("test router must be backed by *store.DB")
	}
	ctx := context.Background()
	raw := `dump of the source failed: pg_dump: error: connection to server at "72.61.8.70", port 5432 failed: timeout expired`
	started := time.Now().Add(-90 * time.Minute)
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "geokit", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}
	if err := db.ClaimDatabaseDataImport(ctx, "geokit", "72.61.8.70", 5432, "src", started, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDatabaseDataImport(ctx, "geokit", store.DataImportFailed, raw, 0, 0, "", started.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	snapshot := func() (copyID string, others map[string]bool, updates updateStatusResource) {
		others = map[string]bool{}
		for _, it := range fetchFeed(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/attention/feed", "")) {
			if it.Kind == attention.KindDataCopy {
				copyID = it.ID
				continue
			}
			others[it.ID] = true
		}
		_, body := suDo(t, rt, cookie, http.MethodGet, "/api/v1/updates", "")
		return copyID, others, decodeBody[updateStatusResource](t, body)
	}
	firstID, othersBefore, updatesBefore := snapshot()
	if firstID == "" || !updatesBefore.UpdateAvailable {
		t.Fatalf("setup: copy item %q, update available %v", firstID, updatesBefore.UpdateAvailable)
	}

	code := dismissItem(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/attention/dismiss", `{"item_key":"`+firstID+`"}`))
	if code != http.StatusNoContent {
		t.Fatalf("dismiss = %d", code)
	}
	if id, _, _ := snapshot(); id != "" {
		t.Fatalf("dismissed copy still listed: %q", id)
	}

	retry := time.Now().Add(-time.Minute)
	if err := db.ClaimDatabaseDataImport(ctx, "geokit", "72.61.8.70", 5432, "src", retry, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDatabaseDataImport(ctx, "geokit", store.DataImportFailed, raw, 0, 0, "", retry.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	secondID, othersAfter, updatesAfter := snapshot()
	if secondID == "" || secondID == firstID {
		t.Fatalf("a new attempt must reappear under a new key: first %q second %q", firstID, secondID)
	}
	if updatesAfter.LatestVersion == nil || updatesBefore.LatestVersion == nil ||
		*updatesAfter.LatestVersion != *updatesBefore.LatestVersion || !updatesAfter.UpdateAvailable {
		t.Fatalf("update status changed: before %+v after %+v", updatesBefore, updatesAfter)
	}
	for id := range othersBefore {
		if !othersAfter[id] {
			t.Errorf("unrelated item %q vanished after the copy was dismissed", id)
		}
	}
}
