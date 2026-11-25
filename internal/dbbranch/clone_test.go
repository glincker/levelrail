package dbbranch

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeCloneStore struct {
	mu     sync.Mutex
	states []string
	reason string
	clone  string
}

func (f *fakeCloneStore) UpdateDatabaseBranchState(_ context.Context, _, from, to, reason, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states = append(f.states, from+">"+to)
	f.reason = reason
	return nil
}
func (f *fakeCloneStore) GetDatabaseBranch(context.Context, string) (store.DatabaseBranch, error) {
	return store.DatabaseBranch{ID: "b1"}, nil
}
func (f *fakeCloneStore) SetDatabaseBranchClone(_ context.Context, _, _, reason, _ string) error {
	f.clone = reason
	return nil
}
func (f *fakeCloneStore) SetDatabaseBranchSize(context.Context, string, int64, string) error {
	return nil
}

type fakeDumper struct {
	err        error
	schemaOnly bool
	body       io.Reader
}

func (d *fakeDumper) Dump(_ context.Context, _, _ string, schemaOnly bool) (io.ReadCloser, string, error) {
	d.schemaOnly = schemaOnly
	if d.body != nil {
		return io.NopCloser(d.body), "best_effort", d.err
	}
	return io.NopCloser(strings.NewReader("dump")), "best_effort", d.err
}

type fakeRestorer struct{ err error }

func (r fakeRestorer) Restore(_ context.Context, _, _ string, dump io.Reader) error {
	_, _ = io.Copy(io.Discard, dump)
	return r.err
}

func newCloner(s *fakeCloneStore, d *fakeDumper, r fakeRestorer, torn *[]string) *Cloner {
	return &Cloner{
		Store: s, Dumper: d, Restorer: r,
		WaitReady: func(context.Context, string) error { return nil },
		Teardown:  func(_ context.Context, n string) error { *torn = append(*torn, n); return nil },
		Container: func(n string) string { return "c-" + n },
	}
}

func TestCloneRunReachesReadyAndLabelsConsistency(t *testing.T) {
	s, d := &fakeCloneStore{}, &fakeDumper{}
	var torn []string
	c := newCloner(s, d, fakeRestorer{}, &torn)
	b := store.DatabaseBranch{ID: "b1", DatabaseName: "br", ParentName: "p", Mode: ModeSchema, State: StateRequested}
	if err := c.Run(context.Background(), b, "postgres"); err != nil {
		t.Fatal(err)
	}
	want := "requested>cloning,cloning>credentials,credentials>verifying,verifying>ready"
	if got := strings.Join(s.states, ","); got != want {
		t.Fatalf("states %s", got)
	}
	if !d.schemaOnly || s.clone != "consistency=best_effort" || len(torn) != 0 {
		t.Fatalf("schema=%v clone=%q torn=%v", d.schemaOnly, s.clone, torn)
	}
}

func TestCloneRunFailureMarksFailedAndCleansUp(t *testing.T) {
	s := &fakeCloneStore{}
	var torn []string
	c := newCloner(s, &fakeDumper{}, fakeRestorer{err: errors.New("boom")}, &torn)
	err := c.Run(context.Background(), store.DatabaseBranch{ID: "b1", DatabaseName: "br", ParentName: "p", State: StateRequested}, "mysql")
	if err == nil || s.states[len(s.states)-1] != "cloning>failed" || len(torn) != 1 || torn[0] != "br" {
		t.Fatalf("err=%v states=%v torn=%v", err, s.states, torn)
	}
}

func TestCloneResetReturnsToReady(t *testing.T) {
	s := &fakeCloneStore{}
	var torn []string
	c := newCloner(s, &fakeDumper{}, fakeRestorer{}, &torn)
	if err := c.Run(context.Background(), store.DatabaseBranch{ID: "b1", DatabaseName: "br", ParentName: "p", State: StateResetting}, "redis"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(s.states, ",") != "resetting>ready" {
		t.Fatalf("states %v", s.states)
	}
}

type fakeExpiryStore struct {
	fakeCloneStore
	rows []store.DatabaseBranch
}

func (f *fakeExpiryStore) ListAllDatabaseBranches(context.Context) ([]store.DatabaseBranch, error) {
	return f.rows, nil
}

func TestExpiryJobDeletesDueAndSkipsProtectedAndParents(t *testing.T) {
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	past, soon := now.Add(-time.Hour).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339)
	fs := &fakeExpiryStore{rows: []store.DatabaseBranch{
		{ID: "1", DatabaseName: "due", ParentName: "p", State: StateReady, ExpiresAt: past},
		{ID: "2", DatabaseName: "prot", ParentName: "p", State: StateReady, ExpiresAt: past, Protected: true},
		{ID: "3", DatabaseName: "parent", ParentName: "p", State: StateReady, ExpiresAt: past},
		{ID: "4", DatabaseName: "kid", ParentName: "parent", State: StateReady, ExpiresAt: soon},
	}}
	var deleted, warned []string
	j := &ExpiryJob{
		Store:  fs,
		Policy: func(context.Context) ExpiryPolicy { return ExpiryPolicy{Enabled: true, Warning: 2 * time.Hour} },
		Delete: func(_ context.Context, b store.DatabaseBranch) error {
			deleted = append(deleted, b.DatabaseName)
			return nil
		},
		Warn: func(_ context.Context, b store.DatabaseBranch, _ time.Time) { warned = append(warned, b.DatabaseName) },
		Now:  func() time.Time { return now },
	}
	j.Tick(context.Background())
	j.Tick(context.Background())
	if len(deleted) != 2 || deleted[0] != "due" {
		t.Fatalf("deleted %v", deleted)
	}
	if len(warned) != 1 || warned[0] != "kid" {
		t.Fatalf("warned once expected, got %v", warned)
	}
}

type errAfterReader struct{ sent bool }

func (r *errAfterReader) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		return copy(p, "partial"), nil
	}
	return 0, errors.New("pq: copy failed on row 7 secret-value")
}

func TestCloneTruncatedDumpFailsWithFixedReason(t *testing.T) {
	s := &fakeCloneStore{}
	var torn []string
	c := newCloner(s, &fakeDumper{body: &errAfterReader{}}, fakeRestorer{}, &torn)
	err := c.Run(context.Background(), store.DatabaseBranch{ID: "b1", DatabaseName: "br", ParentName: "p", State: StateRequested}, "postgres")
	if err == nil || strings.Contains(err.Error(), "secret-value") || strings.Contains(s.reason, "secret-value") {
		t.Fatalf("err=%v reason=%q", err, s.reason)
	}
	if s.reason != "clone failed at step dump (postgres)" || len(torn) != 1 || s.states[len(s.states)-1] != "cloning>failed" {
		t.Fatalf("reason=%q torn=%v states=%v", s.reason, torn, s.states)
	}
}

func TestCloneEmptyBranchFailsVerify(t *testing.T) {
	s := &fakeCloneStore{}
	var torn []string
	c := newCloner(s, &fakeDumper{}, fakeRestorer{}, &torn)
	c.Count = func(_ context.Context, _, container string) (int64, error) {
		if container == "c-p" {
			return 5, nil
		}
		return 0, nil
	}
	err := c.Run(context.Background(), store.DatabaseBranch{ID: "b1", DatabaseName: "br", ParentName: "p", State: StateRequested}, "redis")
	if err == nil || s.reason != "clone failed at step verify (redis)" {
		t.Fatalf("err=%v reason=%q", err, s.reason)
	}
}
