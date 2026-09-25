package pipeline

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	syncYAMLA = "version: 1\nname: ci\njobs:\n  t:\n    image: alpine\n    steps:\n      - run: echo a\n"
	syncYAMLB = "version: 1\nname: ci\njobs:\n  t:\n    image: alpine\n    steps:\n      - run: echo b\n"
)

type fakeFetcher struct {
	files RepoFiles
	err   error
	calls int
	got   struct{ url, token, branch string }
}

func (f *fakeFetcher) FetchFiles(_ context.Context, url, token, branch, _ string, _ []string) (RepoFiles, error) {
	f.calls++
	f.got.url, f.got.token, f.got.branch = url, token, branch
	return f.files, f.err
}

type syncSource struct{ err error }

func (f syncSource) RepoInfo(context.Context, string) (string, string, error) {
	return "https://example.test/r.git", "tok", f.err
}

type syncHarness struct {
	db  *store.DB
	fx  *fakeFetcher
	syn *Syncer
}

func newSyncHarness(t *testing.T, branch string) *syncHarness {
	t.Helper()
	db := openStore(t)
	if err := db.SaveGitSource(context.Background(), store.GitSource{ServiceName: "web", RepoURL: "https://example.test/r.git", Branch: branch, BuildType: "dockerfile"}); err != nil {
		t.Fatal(err)
	}
	fx := &fakeFetcher{}
	return &syncHarness{db: db, fx: fx, syn: NewSyncer(SyncConfig{Store: db, Source: syncSource{}, Fetcher: fx, BrandName: "x"})}
}

func (h *syncHarness) set(sha string, files map[string]string) {
	h.fx.files = RepoFiles{SHA: sha, Dir: ".pipelines", Files: map[string][]byte{}}
	for k, v := range files {
		h.fx.files.Files[k] = []byte(v)
	}
}

func (h *syncHarness) ci(t *testing.T) store.Pipeline {
	t.Helper()
	p, err := h.db.GetPipelineByName(context.Background(), "web", "ci")
	if err != nil {
		t.Fatalf("get ci: %v", err)
	}
	return p
}

func outcomes(r SyncResult) map[string]string {
	m := map[string]string{}
	for _, it := range r.Items {
		m[it.File] = it.Outcome
	}
	return m
}

func TestDecideSync(t *testing.T) {
	hashA, hashB := HashYAML(syncYAMLA), HashYAML(syncYAMLB)
	synced := func(yaml, hash string) *store.Pipeline {
		return &store.Pipeline{Source: SourceRepo, YAML: yaml, SyncedHash: hash}
	}
	tests := []struct {
		name     string
		existing *store.Pipeline
		hash     string
		truth    bool
		want     syncAction
	}{
		{"new", nil, hashA, false, actionCreate},
		{"same and synced", synced(syncYAMLA, hashA), hashA, false, actionUnchanged},
		{"repo changed, untouched locally", synced(syncYAMLA, hashA), hashB, false, actionUpdate},
		{"edited locally, repo unchanged", synced(syncYAMLB, hashA), hashA, false, actionDiverged},
		{"edited locally and repo moved", synced(syncYAMLB, hashA), HashYAML("other"), false, actionDiverged},
		{"edited locally, repo is truth", synced(syncYAMLB, hashA), HashYAML("other"), true, actionUpdate},
		{"ui pipeline with different yaml", &store.Pipeline{Source: "ui", YAML: syncYAMLB}, hashA, false, actionDiverged},
		{"ui pipeline, repo is truth", &store.Pipeline{Source: "ui", YAML: syncYAMLB}, hashA, true, actionUpdate},
		{"ui pipeline with identical yaml is adopted", &store.Pipeline{Source: "ui", YAML: syncYAMLA}, hashA, false, actionUpdate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decideSync(tt.existing, tt.hash, tt.truth); got != tt.want {
				t.Errorf("decideSync = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSyncCreatesUpdatesAndRecordsSHA(t *testing.T) {
	h := newSyncHarness(t, "main")
	ctx := context.Background()

	h.set("sha1", map[string]string{"ci.yaml": syncYAMLA})
	res, err := h.syn.Sync(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if got := outcomes(res)["ci.yaml"]; got != SyncCreated {
		t.Fatalf("first sync outcome = %q", got)
	}
	if h.fx.got.url == "" || h.fx.got.token != "tok" || h.fx.got.branch != "main" {
		t.Fatalf("fetch args = %+v", h.fx.got)
	}
	p := h.ci(t)
	if p.Source != SourceRepo || p.SourceSHA != "sha1" || p.SyncedHash != HashYAML(syncYAMLA) || !p.Enabled {
		t.Fatalf("stored = %+v", p)
	}

	res, _ = h.syn.Sync(ctx, "web")
	if got := outcomes(res)["ci.yaml"]; got != SyncUnchanged {
		t.Fatalf("repeat sync outcome = %q", got)
	}

	h.set("sha2", map[string]string{"ci.yaml": syncYAMLB})
	res, _ = h.syn.Sync(ctx, "web")
	if got := outcomes(res)["ci.yaml"]; got != SyncUpdated {
		t.Fatalf("changed sync outcome = %q", got)
	}
	p = h.ci(t)
	if p.YAML != syncYAMLB || p.SourceSHA != "sha2" || Diverged(p) {
		t.Fatalf("after update = %+v diverged=%v", p, Diverged(p))
	}

	state, _ := h.db.GetPipelineSync(ctx, "web")
	if state.LastSHA != "sha2" || state.LastError != "" || state.LastSyncAt.IsZero() {
		t.Fatalf("state = %+v", state)
	}
}

func TestSyncKeepsDivergedUnlessRepoIsTruth(t *testing.T) {
	h := newSyncHarness(t, "main")
	ctx := context.Background()
	h.set("sha1", map[string]string{"ci.yaml": syncYAMLA})
	if _, err := h.syn.Sync(ctx, "web"); err != nil {
		t.Fatal(err)
	}

	p := h.ci(t)
	p.YAML = strings.Replace(syncYAMLA, "echo a", "echo edited", 1)
	if _, err := h.db.SavePipeline(ctx, p); err != nil {
		t.Fatal(err)
	}
	if !Diverged(h.ci(t)) {
		t.Fatal("edited pipeline should be diverged")
	}

	h.set("sha2", map[string]string{"ci.yaml": syncYAMLB})
	res, _ := h.syn.Sync(ctx, "web")
	if got := outcomes(res)["ci.yaml"]; got != SyncDiverged {
		t.Fatalf("outcome = %q, want diverged", got)
	}
	if !strings.Contains(h.ci(t).YAML, "echo edited") {
		t.Fatal("diverged definition was overwritten")
	}

	if err := h.db.SavePipelineSync(ctx, store.PipelineSync{AppName: "web", RepoIsTruth: true}); err != nil {
		t.Fatal(err)
	}
	res, _ = h.syn.Sync(ctx, "web")
	if got := outcomes(res)["ci.yaml"]; got != SyncUpdated {
		t.Fatalf("outcome with repo is truth = %q", got)
	}
	if got := h.ci(t); got.YAML != syncYAMLB || Diverged(got) {
		t.Fatalf("after overwrite = %+v", got)
	}
}

func TestSyncRejectsBadFiles(t *testing.T) {
	h := newSyncHarness(t, "main")
	other := strings.Replace(strings.Replace(syncYAMLA, "name: ci", "name: other", 1), "run: echo a", "uses: deploy\n        with:\n          service: other-app", 1)
	h.set("sha1", map[string]string{
		"a.yaml":    syncYAMLA,
		"b.yaml":    syncYAMLA,
		"bad.yaml":  "version: 1\njobs: [",
		"other.yml": other,
		"big.yaml":  syncYAMLA + strings.Repeat("# pad\n", maxSyncFileSize/6+1),
	})
	res, err := h.syn.Sync(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	got := outcomes(res)
	want := map[string]string{"a.yaml": SyncCreated, "b.yaml": SyncInvalid, "bad.yaml": SyncInvalid, "other.yml": SyncRefused, "big.yaml": SyncInvalid}
	for f, w := range want {
		if got[f] != w {
			t.Errorf("%s = %q, want %q (items %+v)", f, got[f], w, res.Items)
		}
	}
	list, _ := h.db.ListPipelines(context.Background(), "web")
	if len(list) != 1 {
		t.Fatalf("saved %d pipelines, want 1", len(list))
	}
}

func TestSyncRecordsFetchError(t *testing.T) {
	h := newSyncHarness(t, "main")
	h.fx.err = errors.New("network down")
	if _, err := h.syn.Sync(context.Background(), "web"); err == nil {
		t.Fatal("want error")
	}
	state, _ := h.db.GetPipelineSync(context.Background(), "web")
	if !strings.Contains(state.LastError, "network down") {
		t.Fatalf("state = %+v", state)
	}
	if _, err := h.syn.Sync(context.Background(), "nogit"); !errors.Is(err, ErrNoRepo) {
		t.Fatalf("no git source err = %v", err)
	}
}

func TestSyncReportsOversizeFilesFromFetcher(t *testing.T) {
	h := newSyncHarness(t, "main")
	h.set("sha1", map[string]string{"ci.yaml": syncYAMLA})
	h.fx.files.Oversize = map[string]int64{"huge.yaml": maxSyncFileSize + 1}
	res, err := h.syn.Sync(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, it := range res.Items {
		if it.File == "huge.yaml" && it.Outcome == SyncInvalid {
			found = true
		}
	}
	if !found {
		t.Fatalf("items = %+v, want huge.yaml invalid", res.Items)
	}
}

func TestSyncOnPushOnlyForTrackedBranch(t *testing.T) {
	tests := []struct {
		name    string
		branch  string
		ref     string
		wantRan bool
	}{
		{"tracked branch", "main", "refs/heads/main", true},
		{"other branch", "main", "refs/heads/dev", false},
		{"tag", "main", "refs/tags/v1", false},
		{"default branch tracks any push", "", "refs/heads/anything", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newSyncHarness(t, tt.branch)
			h.set("sha1", map[string]string{"ci.yaml": syncYAMLA})
			_, ran, err := h.syn.SyncOnPush(context.Background(), "web", tt.ref, "sha1")
			if err != nil || ran != tt.wantRan {
				t.Fatalf("ran=%v err=%v, want ran=%v", ran, err, tt.wantRan)
			}
			if (h.fx.calls == 1) != tt.wantRan {
				t.Fatalf("fetch calls = %d", h.fx.calls)
			}
		})
	}
	h := newSyncHarness(t, "main")
	if _, ran, err := h.syn.SyncOnPush(context.Background(), "nogit", "refs/heads/main", "sha1"); ran || err != nil {
		t.Fatalf("app without repo: ran=%v err=%v", ran, err)
	}
}

func TestGitFetcherReadsPipelineDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available for the file transport")
	}
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	write := func(rel, body string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(".pipelines/ci.yaml", syncYAMLA)
	write(".pipelines/notes.txt", "ignored")
	write(".pipelines/nested/x.yaml", "ignored")
	write("README.md", "hi")
	wt, _ := repo.Worktree()
	if _, err := wt.Add("."); err != nil {
		t.Fatal(err)
	}
	hash, err := wt.Commit("init", &git.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@example.test", When: time.Now()}})
	if err != nil {
		t.Fatal(err)
	}

	got, err := GitFetcher{}.FetchFiles(context.Background(), dir, "", "", "", []string{".ci/pipelines", ".pipelines"})
	if err != nil {
		t.Fatal(err)
	}
	if got.SHA != hash.String() || got.Dir != ".pipelines" || len(got.Files) != 1 || string(got.Files["ci.yaml"]) != syncYAMLA {
		t.Fatalf("got = %+v", got)
	}

	none, err := GitFetcher{}.FetchFiles(context.Background(), dir, "", "", "", []string{".missing"})
	if err != nil || len(none.Files) != 0 || none.SHA == "" {
		t.Fatalf("no dir: %+v err %v", none, err)
	}
}
