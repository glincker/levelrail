package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestStaticSiteDeploys_ListAndPromote(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, inspectResolver{})
	root := t.TempDir()
	rt.staticRootDir = root
	rt.publicHost = "8.8.8.8"
	seedApp(t, db, "web")
	old := filepath.Join(root, "web", "abc")
	if err := os.MkdirAll(old, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "index.html"), []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	stats, err := deploy.HashStaticTree(old)
	if err != nil {
		t.Fatal(err)
	}
	seedTarget(t, db, store.DeployAttempt{ID: "dep_AbCd1234xyz", Image: "web:abc", CommitSHA: "abc", ImageDigest: stats.Digest, DigestReason: store.DigestReasonStaticTree})
	current := filepath.Join(root, "web", "def")
	if err := os.MkdirAll(current, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveStaticSite(context.Background(), store.StaticSite{Name: "web", Domains: []string{"web.example.com"}, RootDir: current}); err != nil {
		t.Fatal(err)
	}

	list := func() []staticDeployEntry {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/static-sites/web/deploys", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
		}
		var got staticDeploysResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got.Deploys
	}
	d := list()
	if len(d) != 1 || !d[0].Promotable || d[0].Live || !strings.Contains(d[0].URL, "abcd1234--web.") {
		t.Fatalf("deploys = %+v, want one promotable deploy with an abcd1234-- url", d)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/static-sites/web/deploys/dep_AbCd1234xyz/promote", ""))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("promote = %d %s", rec.Code, rec.Body.String())
	}
	site, err := db.GetStaticSite(context.Background(), "web")
	if err != nil || site.RootDir != old {
		t.Fatalf("site = %+v err=%v, want root %s", site, err, old)
	}
	live := 0
	for _, e := range list() {
		if e.Live && e.ID == "dep_AbCd1234xyz" && !e.Promotable {
			live++
		}
	}
	if live != 1 {
		t.Errorf("want the promoted deploy marked live exactly once")
	}
}
