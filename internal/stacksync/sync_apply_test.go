package stacksync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

type applies struct {
	commits []string
	out     ApplyOutcome
	err     error
}

func (a *applies) apply(_ context.Context, _ store.StackSyncRecord, _ []File, commit string) (ApplyOutcome, error) {
	a.commits = append(a.commits, commit)
	if a.err != nil {
		return ApplyOutcome{}, a.err
	}
	out := a.out
	if out.Revision == 0 {
		out.Revision = len(a.commits)
	}
	return out, nil
}

func applySyncer(t *testing.T, src string, rec store.StackSyncRecord, a *applies) (*Syncer, *memStore, *events) {
	t.Helper()
	ms := &memStore{recs: []store.StackSyncRecord{rec}}
	ev := &events{}
	return &Syncer{Store: ms, Fetch: cloneFetch(src), Plan: pendingPlan, Apply: a.apply, Emit: ev.emit}, ms, ev
}

func tick(t *testing.T, s *Syncer, ms *memStore) store.StackSyncStatus {
	t.Helper()
	recs, _ := ms.ListEnabledStackSyncs(t.Context())
	return s.SyncOne(t.Context(), recs[0])
}

func TestStackSyncApplyOffPlansOnly(t *testing.T) {
	src, _ := fixtureRepo(t, map[string]string{"a.yaml": "x: 1"}, nil)
	a := &applies{}
	s, ms, _ := applySyncer(t, src, syncRec(""), a)
	if st := tick(t, s, ms); st.Code != "" || !st.Pending || len(a.commits) != 0 {
		t.Fatalf("apply off = %+v, applies %d", st, len(a.commits))
	}
}

func TestStackSyncApplyOncePerCommit(t *testing.T) {
	src, head := fixtureRepo(t, map[string]string{"a.yaml": "x: 1"}, nil)
	rec := syncRec("")
	rec.Config.Apply = true
	a := &applies{}
	s, ms, _ := applySyncer(t, src, rec, a)
	st := tick(t, s, ms)
	if st.Code != "" || st.AppliedSHA != head || st.Revision != 1 || st.Pending {
		t.Fatalf("first tick = %+v", st)
	}
	tick(t, s, ms)
	if len(a.commits) != 1 {
		t.Fatalf("same commit applied %d times", len(a.commits))
	}
	writeCommit(t, src, "b.yaml", "y: 2")
	if st := tick(t, s, ms); st.Revision != 2 || len(a.commits) != 2 || a.commits[0] == a.commits[1] {
		t.Fatalf("second commit = %+v %v", st, a.commits)
	}
}

func TestStackSyncApprovalAskedOnce(t *testing.T) {
	src, _ := fixtureRepo(t, map[string]string{"a.yaml": "x: 1"}, nil)
	rec := syncRec("")
	rec.Config.Apply = true
	a := &applies{out: ApplyOutcome{Revision: 4, PendingApproval: true}}
	s, ms, _ := applySyncer(t, src, rec, a)
	if st := tick(t, s, ms); !st.Pending || st.Revision != 4 {
		t.Fatalf("approval tick = %+v", st)
	}
	tick(t, s, ms)
	if len(a.commits) != 1 {
		t.Fatalf("approval requested %d times", len(a.commits))
	}
}

func TestStackSyncApplyFailureCoded(t *testing.T) {
	src, _ := fixtureRepo(t, map[string]string{"a.yaml": "x: 1"}, nil)
	rec := syncRec("")
	rec.Config.Apply = true
	a := &applies{err: errors.New("boom " + canary)}
	s, ms, ev := applySyncer(t, src, rec, a)
	st := tick(t, s, ms)
	if st.Code != iac.CodeSyncApplyFailed || st.AppliedSHA != "" || len(ev.got) != 2 || ev.got[1] != EventFailed {
		t.Fatalf("failed apply = %+v %v", st, ev.got)
	}
	if strings.Contains(strings.Join(ev.raw, ""), canary) {
		t.Fatal("apply error text reached the event")
	}
}

func TestStackSyncDiskLowRefusesFetch(t *testing.T) {
	for name, probe := range map[string]func(context.Context) (int64, error){
		"low":   func(context.Context) (int64, error) { return 1 << 20, nil },
		"error": func(context.Context) (int64, error) { return 0, errors.New("statfs") },
	} {
		fetched := false
		s := &Syncer{Store: &memStore{}, Plan: pendingPlan, FreeBytes: probe,
			Fetch: func(context.Context, store.StackSyncRecord) (string, string, func(), error) {
				fetched = true
				return "", "", nil, errors.New("unreachable")
			}}
		if st := s.SyncOne(t.Context(), syncRec("")); st.Code != iac.CodeSyncDiskLow || fetched {
			t.Fatalf("%s: disk probe = %+v fetched=%v", name, st, fetched)
		}
	}
}

func TestStackSyncOversizeCheckoutDeleted(t *testing.T) {
	src, _ := fixtureRepo(t, map[string]string{"a.yaml": strings.Repeat("x", 4096)}, nil)
	var dir string
	inner := cloneFetch(src)
	s := &Syncer{Store: &memStore{}, Plan: pendingPlan, Limits: func(context.Context) Limits { return Limits{MaxRepoBytes: 1024} },
		Fetch: func(ctx context.Context, rec store.StackSyncRecord) (string, string, func(), error) {
			d, c, cl, err := inner(ctx, rec)
			dir = d
			return d, c, cl, err
		}}
	if st := s.SyncOne(t.Context(), syncRec("")); st.Code != iac.CodeSyncLimitsExceeded {
		t.Fatalf("oversize = %+v", st)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("oversize checkout still on disk: %v", err)
	}
}

func TestStackSyncSecretCodeCarriesLineOnly(t *testing.T) {
	src, _ := fixtureRepo(t, map[string]string{"a.yaml": "token: " + canary}, nil)
	ms, ev := &memStore{}, &events{}
	s := &Syncer{Store: ms, Fetch: cloneFetch(src), Emit: ev.emit,
		Plan: func(context.Context, store.StackSyncRecord, []File) (iac.Plan, error) {
			return iac.Plan{}, &CodedError{Code: iac.CodeSyncSecretInRepo, Msg: "a document holds a literal secret", Detail: "a.yaml:1"}
		}}
	st := s.SyncOne(t.Context(), syncRec(""))
	if st.Code != iac.CodeSyncSecretInRepo || st.Detail != "a.yaml:1" || len(ev.got) != 1 {
		t.Fatalf("secret status = %+v %v", st, ev.got)
	}
	if strings.Contains(strings.Join(ev.raw, "")+st.Detail, canary) {
		t.Fatal("secret value reached status or events")
	}
}

func writeCommit(t *testing.T, dir, name, body string) {
	t.Helper()
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	wt, _ := repo.Worktree()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add(name); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("next", &git.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@example.invalid", When: time.Now()}}); err != nil {
		t.Fatal(err)
	}
}
