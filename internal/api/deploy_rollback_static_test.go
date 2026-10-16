package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRollbackTo_StaticSite(t *testing.T) {
	tests := []struct {
		name   string
		tamper bool
		remove bool
		want   int
	}{
		{"republishes the recorded tree", false, false, http.StatusAccepted},
		{"files changed since the deploy", true, false, http.StatusConflict},
		{"tree removed from disk", false, true, http.StatusGone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db, cookie := newSafetyRouter(t, inspectResolver{})
			root := t.TempDir()
			rt.staticRootDir = root
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
			seedTarget(t, db, store.DeployAttempt{ID: "dep_old", Image: "web:abc", CommitSHA: "abc", ImageDigest: stats.Digest, DigestReason: store.DigestReasonStaticTree})
			current := filepath.Join(root, "web", "def")
			if err := db.SaveStaticSite(context.Background(), store.StaticSite{Name: "web", Domains: []string{"web.example.com"}, RootDir: current}); err != nil {
				t.Fatal(err)
			}
			if tt.tamper {
				_ = os.WriteFile(filepath.Join(old, "index.html"), []byte("v2"), 0o600)
			}
			if tt.remove {
				_ = os.RemoveAll(old)
			}

			rec := rollbackTo(rt, cookie, t, "dep_old", "")
			if rec.Code != tt.want {
				t.Fatalf("rollback = %d %s, want %d", rec.Code, rec.Body.String(), tt.want)
			}
			site, err := db.GetStaticSite(context.Background(), "web")
			if err != nil {
				t.Fatal(err)
			}
			wantRoot := current
			if tt.want == http.StatusAccepted {
				wantRoot = old
			}
			if site.RootDir != wantRoot || len(site.Domains) != 1 {
				t.Errorf("site = %+v, want root %s with its domains kept", site, wantRoot)
			}
			if tt.want != http.StatusAccepted {
				return
			}
			attempts, _ := db.ListDeployAttempts(context.Background(), "web")
			if len(attempts) != 2 || attempts[0].Status != store.DeployAttemptStatusSucceeded ||
				attempts[0].RolloutState != store.RolloutStateServing || attempts[0].ImageDigest != stats.Digest {
				t.Errorf("attempts = %+v, want a succeeded serving rollback attempt", attempts)
			}
		})
	}
}
