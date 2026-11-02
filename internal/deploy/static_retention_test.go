package deploy

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeRetentionStore struct {
	attempts []store.DeployAttempt
	sites    []store.StaticSite
	lock     *store.PublishLock
}

func (f *fakeRetentionStore) ListDeployAttempts(context.Context, string) ([]store.DeployAttempt, error) {
	return f.attempts, nil
}

func (f *fakeRetentionStore) ListStaticSites(context.Context) ([]store.StaticSite, error) {
	return f.sites, nil
}

func (f *fakeRetentionStore) GetPublishLock(context.Context, string) (*store.PublishLock, error) {
	return f.lock, nil
}

// makeReleases creates names oldest first, one minute apart.
func makeReleases(t *testing.T, root, svc string, names ...string) {
	t.Helper()
	base := time.Now().Add(-time.Hour)
	for i, n := range names {
		dir := filepath.Join(root, svc, n)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(n), 0o600); err != nil {
			t.Fatal(err)
		}
		at := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(dir, at, at); err != nil {
			t.Fatal(err)
		}
	}
}

func remaining(t *testing.T, root, svc string) []string {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join(root, svc))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range ents {
		out = append(out, e.Name())
	}
	slices.Sort(out)
	return out
}

func liveAt(root, rel string) store.StaticSite {
	return store.StaticSite{Name: "site", RootDir: filepath.Join(root, "site", rel)}
}

func TestRetentionKeepsNewestN(t *testing.T) {
	root := t.TempDir()
	makeReleases(t, root, "site", "r1", "r2", "r3", "r4", "r5", "r6")
	fs := &fakeRetentionStore{sites: []store.StaticSite{liveAt(root, "r6")}}
	removed, err := PruneStaticReleases(context.Background(), root, "site", 4, fs)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Fatalf("removed = %v", removed)
	}
	if got := remaining(t, root, "site"); !slices.Equal(got, []string{"r3", "r4", "r5", "r6"}) {
		t.Fatalf("remaining = %v", got)
	}
}

func TestRetentionRespectsMinimum(t *testing.T) {
	root := t.TempDir()
	makeReleases(t, root, "site", "r1", "r2", "r3", "r4", "r5")
	fs := &fakeRetentionStore{sites: []store.StaticSite{liveAt(root, "r5")}}
	if _, err := PruneStaticReleases(context.Background(), root, "site", 1, fs); err != nil {
		t.Fatal(err)
	}
	if got := remaining(t, root, "site"); len(got) != 3 {
		t.Fatalf("remaining = %v, want 3", got)
	}
	t.Setenv("APP_STATIC_KEEP_RELEASES", "1")
	if StaticKeepReleases() != 3 {
		t.Fatal("env below the minimum not raised")
	}
	t.Setenv("APP_STATIC_KEEP_RELEASES", "")
	if StaticKeepReleases() != 10 {
		t.Fatal("default is not 10")
	}
}

func TestRetentionKeepsLiveAndPrevious(t *testing.T) {
	root := t.TempDir()
	makeReleases(t, root, "site", "old1", "prev", "old2", "old3", "live", "newer")
	fs := &fakeRetentionStore{
		sites: []store.StaticSite{liveAt(root, "live")},
		attempts: []store.DeployAttempt{
			{CommitSHA: "live", Status: store.DeployAttemptStatusSucceeded, ImageDigest: "d3"},
			{CommitSHA: "prev", Status: store.DeployAttemptStatusSucceeded, ImageDigest: "d2"},
			{CommitSHA: "old1", Status: store.DeployAttemptStatusSucceeded, ImageDigest: "d1"},
		},
	}
	if _, err := PruneStaticReleases(context.Background(), root, "site", 3, fs); err != nil {
		t.Fatal(err)
	}
	got := remaining(t, root, "site")
	for _, want := range []string{"live", "prev", "newer"} {
		if !slices.Contains(got, want) {
			t.Fatalf("%s was removed, remaining %v", want, got)
		}
	}
	if slices.Contains(got, "old2") {
		t.Fatalf("old2 should be pruned: %v", got)
	}
}

func TestRetentionKeepsPendingAttemptReleases(t *testing.T) {
	root := t.TempDir()
	names := []string{"held", "await", "queued", "running", "gone", "r6", "r7", "r8", "live"}
	makeReleases(t, root, "site", names...)
	var attempts []store.DeployAttempt
	for name, status := range map[string]string{
		"held": store.DeployAttemptStatusHeld, "await": store.DeployAttemptStatusAwaitingApproval,
		"queued": store.DeployAttemptStatusQueued, "running": store.DeployAttemptStatusRunning,
		"gone": store.DeployAttemptStatusFailed,
	} {
		attempts = append(attempts, store.DeployAttempt{CommitSHA: name, Status: status})
	}
	fs := &fakeRetentionStore{sites: []store.StaticSite{liveAt(root, "live")}, attempts: attempts}
	if _, err := PruneStaticReleases(context.Background(), root, "site", 3, fs); err != nil {
		t.Fatal(err)
	}
	got := remaining(t, root, "site")
	for _, want := range []string{"held", "await", "queued", "running", "live"} {
		if !slices.Contains(got, want) {
			t.Fatalf("%s was removed, remaining %v", want, got)
		}
	}
	if slices.Contains(got, "gone") {
		t.Fatalf("a failed attempt must not protect its release: %v", got)
	}
}

func TestRetentionKeepsReleaseServedByAnotherSite(t *testing.T) {
	root := t.TempDir()
	makeReleases(t, root, "site", "shared", "b", "c", "d", "live")
	fs := &fakeRetentionStore{sites: []store.StaticSite{
		liveAt(root, "live"),
		{Name: "site-preview", RootDir: filepath.Join(root, "site", "shared")},
	}}
	if _, err := PruneStaticReleases(context.Background(), root, "site", 3, fs); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(remaining(t, root, "site"), "shared") {
		t.Fatal("a release another site serves was removed")
	}
}

func TestRetentionDoesNotFollowSymlinkOutside(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	keepFile := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(keepFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	makeReleases(t, root, "site", "a", "b", "c", "d", "live")
	if err := os.Symlink(outside, filepath.Join(root, "site", "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "site", "a", "inner")); err != nil {
		t.Fatal(err)
	}
	fs := &fakeRetentionStore{sites: []store.StaticSite{liveAt(root, "live")}}
	if _, err := PruneStaticReleases(context.Background(), root, "site", 3, fs); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keepFile); err != nil {
		t.Fatalf("file outside the releases dir was touched: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "site", "linked")); err != nil {
		t.Fatalf("symlink entry should be left alone: %v", err)
	}
}

func TestRetentionRefusesSiteDirOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	makeReleases(t, outside, "x", "a", "b", "c", "d", "e")
	if err := os.Symlink(filepath.Join(outside, "x"), filepath.Join(root, "site")); err != nil {
		t.Fatal(err)
	}
	if _, err := PruneStaticReleases(context.Background(), root, "site", 3, &fakeRetentionStore{}); err == nil {
		t.Fatal("site dir outside the root was accepted")
	}
	if got := remaining(t, outside, "x"); len(got) != 5 {
		t.Fatalf("outside releases touched: %v", got)
	}
}

func TestRetentionIsIdempotent(t *testing.T) {
	root := t.TempDir()
	makeReleases(t, root, "site", "r1", "r2", "r3", "r4", "r5", "r6")
	fs := &fakeRetentionStore{sites: []store.StaticSite{liveAt(root, "r6")}}
	if _, err := PruneStaticReleases(context.Background(), root, "site", 3, fs); err != nil {
		t.Fatal(err)
	}
	removed, err := PruneStaticReleases(context.Background(), root, "site", 3, fs)
	if err != nil || len(removed) != 0 {
		t.Fatalf("second run removed %v, err %v", removed, err)
	}
	if got := remaining(t, root, "site"); len(got) != 3 {
		t.Fatalf("remaining = %v", got)
	}
}

func TestRetentionSkipsWhileLockHeld(t *testing.T) {
	root := t.TempDir()
	makeReleases(t, root, "site", "r1", "r2", "r3", "r4", "r5", "r6")
	fs := &fakeRetentionStore{
		sites: []store.StaticSite{liveAt(root, "r6")},
		lock:  &store.PublishLock{},
	}
	removed, err := PruneStaticReleases(context.Background(), root, "site", 3, fs)
	if err != nil || len(removed) != 0 {
		t.Fatalf("removed %v, err %v", removed, err)
	}
	if got := remaining(t, root, "site"); len(got) != 6 {
		t.Fatalf("remaining = %v", got)
	}
}
