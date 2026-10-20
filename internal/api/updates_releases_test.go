package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

func historyRouter(t *testing.T) (*Router, *http.Cookie, int) {
	t.Helper()
	setVersion(t, "v1.5.0")
	rt, db := newTestRouter(t)
	dir := t.TempDir()
	live, err := store.Open(context.Background(), filepath.Join(dir, liveDBFile))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = live.Close() })
	rt.dataDir = dir
	schema, err := store.MaxSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	manifests := map[string]int{
		"https://github.com/m/v1.4.0": schema,
		"https://github.com/m/v1.0.0": schema - 3,
	}
	rt.releaseHist.list = func(context.Context, string) ([]upgrade.HistoryRelease, error) {
		mk := func(tag, date string, withManifest bool) upgrade.HistoryRelease {
			r := upgrade.HistoryRelease{Tag: tag, PublishedAt: date, Body: "<!-- x -->\n> [!NOTE]\n> " + tag,
				Assets: []upgrade.HistoryAsset{{Name: rtAsset(rt), Size: 1234}, {Name: upgrade.SignatureAsset}}}
			if withManifest {
				r.Assets = append(r.Assets, upgrade.HistoryAsset{Name: upgrade.ManifestAsset, URL: "https://github.com/m/" + tag})
			}
			return r
		}
		return []upgrade.HistoryRelease{
			mk("v1.5.0", "2026-10-03T00:00:00Z", false),
			mk("v1.4.0", "2026-10-02T00:00:00Z", true),
			mk("v1.0.0", "2026-09-01T00:00:00Z", true),
		}, nil
	}
	rt.releaseHist.manifest = func(_ context.Context, u string) (upgrade.Manifest, error) {
		return upgrade.Manifest{SchemaVersion: manifests[u]}, nil
	}
	return rt, loginTestSession(t, rt, db), schema
}

func rtAsset(rt *Router) string { return rt.assetName() }

func TestHandleReleaseHistory_VerdictsPerRelease(t *testing.T) {
	rt, cookie, schema := historyRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/updates/releases?channel=all", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got releaseHistoryResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.CurrentSchemaVersion == nil || *got.CurrentSchemaVersion != schema {
		t.Fatalf("current schema = %v, want %d", got.CurrentSchemaVersion, schema)
	}
	want := map[string]upgrade.SchemaVerdict{
		"v1.5.0": upgrade.VerdictUnknown,
		"v1.4.0": upgrade.VerdictBinaryOnly,
		"v1.0.0": upgrade.VerdictRestoreRequired,
	}
	for _, r := range got.Releases {
		if r.Verdict != want[r.Version] {
			t.Errorf("%s verdict = %s, want %s", r.Version, r.Verdict, want[r.Version])
		}
		if r.Notes != "> [!NOTE]\n> "+r.Version {
			t.Errorf("%s notes not cleaned: %q", r.Version, r.Notes)
		}
	}
	if !got.Releases[0].Running || got.Releases[1].Running {
		t.Errorf("running flag wrong: %+v", got.Releases)
	}
}

func TestHandleRollbackPlan_RestoreRequiredIsExplicit(t *testing.T) {
	rt, cookie, _ := historyRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/updates/rollback-plan?version=v1.0.0", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got rollbackPlanResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Verdict != upgrade.VerdictRestoreRequired || !got.RestoreRequired {
		t.Fatalf("plan = %+v", got.Plan)
	}
	if got.FetchCommand == "" {
		t.Error("a release that is not retained needs a fetch command")
	}
	if len(got.Changes) == 0 {
		t.Error("expected release notes between v1.0.0 and the running version")
	}
}

func TestHandleRollbackPlan_Validation(t *testing.T) {
	rt, cookie, _ := historyRouter(t)
	for _, tc := range []struct {
		query string
		want  int
	}{
		{"version=../../etc/passwd", http.StatusBadRequest},
		{"version=v9.9.9", http.StatusNotFound},
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/updates/rollback-plan?"+tc.query, ""))
		if rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d", tc.query, rec.Code, tc.want)
		}
	}
}
