package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

func schemaTestRouter(t *testing.T, current string, fetch ReleaseSchemaFetcher, assets ...string) (*Router, *http.Cookie, int) {
	t.Helper()
	setVersion(t, current)
	release := releaseWithAssets(assets...)
	rt, db := newTestRouterWithFetchLatestRelease(t, func(context.Context) (*githubRelease, error) { return release, nil })
	dir := t.TempDir()
	live, err := store.Open(context.Background(), filepath.Join(dir, liveDBFile))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = live.Close() })
	rt.dataDir = dir
	WithReleaseSchemaSource(fetch)(rt)
	dbSchema, err := store.MaxSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	return rt, loginTestSession(t, rt, db), dbSchema
}

func getPreflight(t *testing.T, rt *Router, cookie *http.Cookie) updatePreflightResource {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/updates/preflight", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got updatePreflightResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func schemaCheckOf(t *testing.T, p updatePreflightResource) upgrade.Check {
	t.Helper()
	for _, c := range p.Checks {
		if c.Code == upgrade.CodeSchemaCompat {
			return c
		}
	}
	t.Fatalf("no %s check in %+v", upgrade.CodeSchemaCompat, p.Checks)
	return upgrade.Check{}
}

func TestPreflightSchemaCompat(t *testing.T) {
	signed := []string{upgrade.ChecksumsAsset, upgrade.SignatureAsset, upgrade.ManifestAsset}
	fixed := func(schema int, err error) ReleaseSchemaFetcher {
		return func(context.Context, string) (selfupgrade.SchemaResult, error) {
			return selfupgrade.SchemaResult{Version: "v9.9.9", Schema: schema}, err
		}
	}
	tests := []struct {
		name          string
		offset        int
		err           error
		assets        []string
		wantStatus    string
		wantAvailable bool
		wantBlocked   bool
	}{
		{"older than the database", -1000, nil, signed, upgrade.StatusFail, false, true},
		{"same schema", 0, nil, signed, upgrade.StatusOK, true, false},
		{"newer schema", 4, nil, signed, upgrade.StatusOK, true, false},
		{"release without a manifest", 0, nil, []string{upgrade.ChecksumsAsset, upgrade.SignatureAsset}, upgrade.StatusUnknown, true, false},
		{"manifest unreachable", 0, errors.New("offline"), signed, upgrade.StatusUnknown, true, false},
		{"manifest fails verification", 0, fmt.Errorf("%w: bad", selfupgrade.ErrSignatureInvalid), signed, upgrade.StatusWarn, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			var dbSchema int
			fetch := func(ctx context.Context, tag string) (selfupgrade.SchemaResult, error) {
				calls++
				return fixed(dbSchema+tc.offset, tc.err)(ctx, tag)
			}
			var rt *Router
			var cookie *http.Cookie
			rt, cookie, dbSchema = schemaTestRouter(t, "v1.0.0", fetch, tc.assets...)
			got := getPreflight(t, rt, cookie)
			c := schemaCheckOf(t, got)
			if c.Status != tc.wantStatus || got.UpdateAvailable != tc.wantAvailable || (tc.wantBlocked && !got.Blocked) {
				t.Fatalf("status %q available %v blocked %v: %+v", c.Status, got.UpdateAvailable, got.Blocked, c)
			}
			if tc.wantStatus == upgrade.StatusFail {
				want := upgrade.DowngradeMessage(dbSchema, "v9.9.9", dbSchema+tc.offset)
				if c.Message != want || c.Fix == nil || c.Fix.Action != upgrade.FixOpenRollback {
					t.Fatalf("message %q fix %+v, want %q with open_rollback", c.Message, c.Fix, want)
				}
				_, status := getUpdates(t, rt, cookie)
				if status.UpdateAvailable {
					t.Fatal("GET /api/v1/updates still offers a release the database cannot accept")
				}
			}
			getPreflight(t, rt, cookie)
			if tc.err == nil && calls > 1 {
				t.Fatalf("manifest fetched %d times, want it cached", calls)
			}
		})
	}
}

func TestPreflightSchemaCompatBlocksDescribeBuildIncident(t *testing.T) {
	fetch := func(context.Context, string) (selfupgrade.SchemaResult, error) {
		return selfupgrade.SchemaResult{Version: "v9.9.9", Schema: 1}, nil
	}
	rt, cookie, _ := schemaTestRouter(t, "v9.9.8-138-gabbfcdba5", fetch, upgrade.ChecksumsAsset, upgrade.SignatureAsset, upgrade.ManifestAsset)
	got := getPreflight(t, rt, cookie)
	if got.UpdateAvailable || !got.Blocked {
		t.Fatalf("available %v blocked %v, want an older-schema release refused before the click", got.UpdateAvailable, got.Blocked)
	}
}

func TestPreflightWithoutSchemaSourceOmitsCheck(t *testing.T) {
	setVersion(t, "v1.0.0")
	rt, db := newTestRouterWithFetchLatestRelease(t, func(context.Context) (*githubRelease, error) {
		return releaseWithAssets(upgrade.ChecksumsAsset, upgrade.SignatureAsset), nil
	})
	got := getPreflight(t, rt, loginTestSession(t, rt, db))
	for _, c := range got.Checks {
		if c.Code == upgrade.CodeSchemaCompat {
			t.Fatal("schema check present without a source")
		}
	}
}
