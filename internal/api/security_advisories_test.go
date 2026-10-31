package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/advisoryfeed"
	"github.com/GLINCKER/levelrail/internal/store"
)

const refreshTestFeed = `{"version":1,"entries":[{"id":"T-1","severity":"critical","summary":"s","fixed_in":"2","affected":[{"repository":"nginx","tags":["1"]}]}]}`

// useAdvisoryFeed swaps the process feed service for one over a temp file.
func useAdvisoryFeed(t *testing.T, feed string, sign bool) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.json")
	if err := os.WriteFile(path, []byte(feed), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := advisoryfeed.Config{Source: path, MaxBytes: 1 << 20}
	if sign {
		pub, priv, _ := ed25519.GenerateKey(rand.Reader)
		cfg.PubKey = base64.StdEncoding.EncodeToString(pub)
		sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(feed)))
		if err := os.WriteFile(path+".sig", []byte(sig), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	prevSvc := advisorySvc
	advisoryOnce.Do(func() {})
	advisorySvc = advisoryfeed.New(cfg, nil)
	t.Cleanup(func() { advisorySvc = prevSvc })
}

func TestAdvisoryListMatchesDeployedApp(t *testing.T) {
	rt, _, cookie := newSafetyRouter(t, stubResolver{digest: "sha256:new"})
	useAdvisoryFeed(t, refreshTestFeed, true)
	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/security/advisories", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var res advisoryResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Matches) != 1 || res.Matches[0].App != "web" || res.Signature != advisoryfeed.SigVerified {
		t.Fatalf("matches = %+v sig=%s", res.Matches, res.Signature)
	}
}

func TestAdvisoryRefreshRedeploysDigestPinned(t *testing.T) {
	t.Setenv(envAdvisoryRefresh, "on")
	rt, db, cookie := newSafetyRouter(t, stubResolver{digest: "sha256:new"})
	useAdvisoryFeed(t, refreshTestFeed, true)
	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/advisory-refresh", `{}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	got, err := db.GetDesiredService(t.Context(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if got.Image != "nginx:1@sha256:new" {
		t.Fatalf("image = %q, want digest pinned", got.Image)
	}
}

func TestAdvisoryRefreshRefusals(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		feed    string
		signed  bool
		resolve stubResolver
		pinned  string
		want    int
	}{
		{"flag off", "", refreshTestFeed, true, stubResolver{digest: "sha256:new"}, "", http.StatusConflict},
		{"unsigned feed", "on", refreshTestFeed, false, stubResolver{digest: "sha256:new"}, "", http.StatusConflict},
		{"no match", "on", `{"version":1,"entries":[{"id":"T-2","severity":"high","summary":"s","affected":[{"repository":"other","tags":["1"]}]}]}`, true, stubResolver{digest: "sha256:new"}, "", http.StatusConflict},
		{"same digest", "on", refreshTestFeed, true, stubResolver{digest: "sha256:cur"}, "nginx:1@sha256:cur", http.StatusConflict},
		{"registry unreachable", "on", refreshTestFeed, true, stubResolver{err: errors.New("down")}, "", http.StatusBadGateway},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envAdvisoryRefresh, tc.flag)
			rt, db, cookie := newSafetyRouter(t, tc.resolve)
			useAdvisoryFeed(t, tc.feed, tc.signed)
			if tc.pinned != "" {
				if err := db.SaveDesiredService(t.Context(), store.DesiredService{Name: "web", Image: tc.pinned, Port: 80}); err != nil {
					t.Fatal(err)
				}
			}
			rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/advisory-refresh", `{}`))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
			got, _ := db.GetDesiredService(t.Context(), "web")
			if tc.pinned == "" && got.Image != "nginx:1" {
				t.Fatalf("a refused refresh changed the image to %q", got.Image)
			}
		})
	}
}
