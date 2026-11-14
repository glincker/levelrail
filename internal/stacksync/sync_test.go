package stacksync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

const canary = "CANARY-sync-credential-7f3a"

// fixtureRepo builds a local repository in the test; nothing touches the network.
func fixtureRepo(t *testing.T, files map[string]string, links map[string]string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, _ := repo.Worktree()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		_ = os.MkdirAll(filepath.Dir(p), 0o750)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for name, target := range links {
		p := filepath.Join(dir, filepath.FromSlash(name))
		_ = os.MkdirAll(filepath.Dir(p), 0o750)
		if err := os.Symlink(target, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := wt.AddGlob("."); err != nil {
		t.Fatal(err)
	}
	h, err := wt.Commit("fixture", &git.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@example.invalid", When: time.Now()}})
	if err != nil {
		t.Fatal(err)
	}
	return dir, h.String()
}

func cloneFetch(src string) Fetch {
	return func(ctx context.Context, _ store.StackSyncRecord) (string, string, func(), error) {
		dst, err := os.MkdirTemp("", "stacksync-test-*")
		if err != nil {
			return "", "", nil, err
		}
		r, err := git.PlainCloneContext(ctx, dst, false, &git.CloneOptions{URL: src})
		if err != nil {
			return "", "", nil, err
		}
		head, _ := r.Head()
		return dst, head.Hash().String(), func() { _ = os.RemoveAll(dst) }, nil
	}
}

type memStore struct {
	mu     sync.Mutex
	recs   []store.StackSyncRecord
	status map[string]store.StackSyncStatus
	lists  int
}

func (m *memStore) ListEnabledStackSyncs(context.Context) ([]store.StackSyncRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lists++
	var out []store.StackSyncRecord
	for _, r := range m.recs {
		if r.Config.Enabled {
			r.Status = m.status[r.StackID]
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memStore) SetStackSyncStatus(_ context.Context, id string, st store.StackSyncStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status == nil {
		m.status = map[string]store.StackSyncStatus{}
	}
	m.status[id] = st
	return nil
}

type events struct {
	mu  sync.Mutex
	got []string
	raw []string
}

func (e *events) emit(_ context.Context, typ string, rec store.StackSyncRecord, st store.StackSyncStatus) {
	e.mu.Lock()
	defer e.mu.Unlock()
	b, _ := json.Marshal(map[string]any{"stack": rec.StackName, "status": st})
	e.got = append(e.got, typ)
	e.raw = append(e.raw, string(b))
}

func pendingPlan(_ context.Context, _ store.StackSyncRecord, files []File) (iac.Plan, error) {
	return iac.Plan{Hash: "h1", Summary: iac.Summary{Create: len(files)}}, nil
}

func syncRec(path string) store.StackSyncRecord {
	return store.StackSyncRecord{StackID: "s1", StackName: "web", Config: store.StackSyncConfig{
		RepoURL: "https://git.example.invalid/org/infra.git", Branch: "main", Path: path, Enabled: true,
	}}
}

func TestStackSyncDisabledDoesNothing(t *testing.T) {
	ms := &memStore{recs: []store.StackSyncRecord{{StackID: "s1", Config: store.StackSyncConfig{Enabled: false}}}}
	fetched := false
	s := &Syncer{Store: ms, Fetch: func(context.Context, store.StackSyncRecord) (string, string, func(), error) {
		fetched = true
		return "", "", nil, errors.New("no")
	}, Plan: pendingPlan}
	s.Start(t.Context())
	deadline := time.Now().Add(2 * time.Second)
	for s.Running() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if s.Running() || fetched {
		t.Fatalf("running=%v fetched=%v, want an idle syncer with no fetch", s.Running(), fetched)
	}
}

func TestStackSyncPlansPendingOncePerCommit(t *testing.T) {
	src, sha := fixtureRepo(t, map[string]string{"infra/app.yaml": "kind: App\n", "README.md": "x"}, nil)
	ms := &memStore{recs: []store.StackSyncRecord{syncRec("infra")}}
	ev := &events{}
	s := &Syncer{Store: ms, Fetch: cloneFetch(src), Plan: pendingPlan, Emit: ev.emit}
	recs, _ := ms.ListEnabledStackSyncs(t.Context())
	st := s.SyncOne(t.Context(), recs[0])
	if st.Code != "" || st.LastSHA != sha || !st.Pending || st.Counts["create"] != 1 || st.PlanHash != "h1" {
		t.Fatalf("status = %+v, want pending plan at %s", st, sha)
	}
	recs, _ = ms.ListEnabledStackSyncs(t.Context())
	s.SyncOne(t.Context(), recs[0])
	if len(ev.got) != 1 || ev.got[0] != EventPending {
		t.Fatalf("events = %v, want one sync_pending for an unchanged commit", ev.got)
	}
}

func TestStackSyncPathAndURLRefused(t *testing.T) {
	for _, c := range []iac.StackSync{
		{RepoURL: "https://h.example/r.git", Branch: "main", Path: "../etc"},
		{RepoURL: "https://h.example/r.git", Branch: "main", Path: "a/../../b"},
		{RepoURL: "https://h.example/r.git", Branch: "main", Path: "/abs"},
		{RepoURL: "https://h.example/r.git", Branch: "main", Path: ".git/hooks"},
	} {
		if code, err := c.Validate(); err == nil || code != iac.CodeSyncPathInvalid {
			t.Errorf("path %q: code %q err %v, want sync_path_invalid", c.Path, code, err)
		}
	}
	for _, u := range []string{"file:///tmp/repo", "/tmp/repo", "./repo", "ext::sh -c id", "-uhost", "https://u:" + canary + "@h.example/r.git", "http://h.example/r.git"} {
		if _, err := (iac.StackSync{RepoURL: u, Branch: "main"}).Validate(); err == nil {
			t.Errorf("repo_url %q accepted", u)
		}
	}
	for _, b := range []string{"-x", "a..b", "a b", "x.lock", ""} {
		if _, err := (iac.StackSync{RepoURL: "git@h.example:org/r.git", Branch: b}).Validate(); err == nil {
			t.Errorf("branch %q accepted", b)
		}
	}
	if _, err := (iac.StackSync{RepoURL: "git@h.example:org/r.git", Branch: "release/v1", Path: "infra/prod", CredentialRef: "app:web"}).Validate(); err != nil {
		t.Fatalf("valid config refused: %v", err)
	}
}

func TestStackSyncLimitsEnforced(t *testing.T) {
	src, _ := fixtureRepo(t, map[string]string{"a.yaml": "kind: App\n", "b.yaml": strings.Repeat("x", 2048)}, nil)
	for name, lim := range map[string]Limits{
		"files": {MaxFiles: 1}, "file size": {MaxFileBytes: 1024}, "repo size": {MaxRepoBytes: 512},
	} {
		ms := &memStore{recs: []store.StackSyncRecord{syncRec("")}}
		l := lim
		s := &Syncer{Store: ms, Fetch: cloneFetch(src), Plan: pendingPlan, Limits: func(context.Context) Limits { return l }}
		if st := s.SyncOne(t.Context(), ms.recs[0]); st.Code != iac.CodeSyncLimitsExceeded {
			t.Errorf("%s: code %q, want sync_limits_exceeded", name, st.Code)
		}
	}
}

func TestStackSyncSymlinkEscapeRefused(t *testing.T) {
	src, _ := fixtureRepo(t, map[string]string{"infra/a.yaml": "kind: App\n"}, map[string]string{"infra/leak.yaml": "../../../../etc/passwd", "linked": "infra"})
	for _, p := range []string{"infra", "linked"} {
		ms := &memStore{recs: []store.StackSyncRecord{syncRec(p)}}
		s := &Syncer{Store: ms, Fetch: cloneFetch(src), Plan: pendingPlan}
		if st := s.SyncOne(t.Context(), ms.recs[0]); st.Code != iac.CodeSyncPathInvalid {
			t.Errorf("path %q: code %q, want sync_path_invalid", p, st.Code)
		}
	}
}

func TestStackSyncCredentialCanaryAbsent(t *testing.T) {
	var logs bytes.Buffer
	rec := syncRec("")
	rec.Config.CredentialRef = "app:web"
	ms := &memStore{recs: []store.StackSyncRecord{rec}}
	ev := &events{}
	s := &Syncer{Store: ms, Emit: ev.emit, Plan: pendingPlan, Logger: slog.New(slog.NewTextHandler(&logs, nil)),
		Fetch: func(context.Context, store.StackSyncRecord) (string, string, func(), error) {
			return "", "", nil, errors.New("auth failed for token " + canary)
		}}
	st := s.SyncOne(t.Context(), ms.recs[0])
	b, _ := json.Marshal(ms.status)
	all := string(b) + strings.Join(ev.raw, "") + logs.String()
	if st.Code != iac.CodeSyncRepoUnreachable || len(ev.got) != 1 || ev.got[0] != EventFailed {
		t.Fatalf("code %q events %v, want one sync_failed with sync_repo_unreachable", st.Code, ev.got)
	}
	if strings.Contains(all, canary) {
		t.Fatal("credential canary leaked into status, events or logs")
	}
}
