package stackdrift

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

const projectDoc = "version: 1\nkind: Project\nmetadata: {name: shop}\nspec:\n  env:\n    REGION: eu\n    FEATURE_SET: declared-value-1\n"

type fakeStore struct {
	stacks   []store.StackRecord
	revs     map[string][]store.StackRevisionRecord
	owners   map[string]string
	panicOn  string
	recorded map[string]store.StackDrift
}

func newStore(names ...string) *fakeStore {
	fs := &fakeStore{revs: map[string][]store.StackRevisionRecord{}, owners: map[string]string{}, recorded: map[string]store.StackDrift{}}
	raw, _ := json.Marshal([]renderedFile{{Name: "stack.yaml", Content: projectDoc}})
	for _, n := range names {
		fs.stacks = append(fs.stacks, store.StackRecord{ID: "id-" + n, Name: n, ProjectID: "p1"})
		fs.revs["id-"+n] = []store.StackRevisionRecord{
			{StackID: "id-" + n, Number: 2, Status: store.StackRevFailed},
			{StackID: "id-" + n, Number: 1, Status: store.StackRevApplied, RenderedDocs: raw},
		}
	}
	return fs
}

func (f *fakeStore) ListStacks(context.Context) ([]store.StackRecord, error) { return f.stacks, nil }
func (f *fakeStore) ListStackRevisions(_ context.Context, id string, _ int) ([]store.StackRevisionRecord, error) {
	if id == f.panicOn {
		panic("boom")
	}
	return f.revs[id], nil
}
func (f *fakeStore) GetStackRevision(_ context.Context, id string, n int) (store.StackRevisionRecord, error) {
	for _, v := range f.revs[id] {
		if v.Number == n {
			return v, nil
		}
	}
	return store.StackRevisionRecord{}, store.ErrStackRevisionNotFound
}
func (f *fakeStore) ListStackLedger(context.Context, string) ([]store.StackLedgerEntry, error) {
	return nil, nil
}
func (f *fakeStore) GetStackResourceOwner(_ context.Context, _, key string) (store.StackLedgerEntry, bool, error) {
	id, ok := f.owners[key]
	return store.StackLedgerEntry{StackID: id, Key: key}, ok, nil
}
func (f *fakeStore) SetStackDrift(_ context.Context, d store.StackDrift) error {
	f.recorded[d.StackID] = d
	return nil
}

// fakeLive answers only reads and fails the test on any write.
type fakeLive struct {
	t       *testing.T
	project bool
	env     map[string]string
	failAll bool
}

func (l *fakeLive) Do(_ context.Context, method, path string, _, out any) error {
	if method != http.MethodGet {
		l.t.Errorf("drift check wrote live state: %s %s", method, path)
		return errors.New("write refused")
	}
	if l.failAll {
		return &iac.StatusError{Status: 500, Message: "live read failed"}
	}
	var v any
	switch path {
	case "/api/v1/projects":
		v = []any{}
		if l.project {
			v = []any{map[string]string{"id": "p1", "name": "shop"}}
		}
	case "/api/v1/tags", "/api/v1/databases":
		v = []any{}
	case "/api/v1/projects/p1/env":
		v = l.env
	default:
		return &iac.StatusError{Status: 404, Message: "not found"}
	}
	raw, _ := json.Marshal(v)
	return json.Unmarshal(raw, out)
}

func checker(t *testing.T, fs *fakeStore, live *fakeLive) *Checker {
	t.Helper()
	return &Checker{
		Store:       fs,
		Live:        func(context.Context) (iac.Doer, func(), error) { return live, func() {}, nil },
		ProjectName: func(context.Context, string) string { return "shop" },
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:         func() time.Time { return time.Unix(1700000000, 0) },
	}
}

func TestCheckAll_NoDrift(t *testing.T) {
	fs := newStore("web")
	live := &fakeLive{t: t, project: true, env: map[string]string{"REGION": "eu", "FEATURE_SET": "declared-value-1"}}
	checker(t, fs, live).CheckAll(t.Context())
	d := fs.recorded["id-web"]
	if d.Status != store.StackDriftNone || len(d.Summary) != 0 || d.Revision != 1 || d.CheckedAt.IsZero() {
		t.Fatalf("drift = %+v", d)
	}
}

func TestCheckAll_ChangedFieldPathWithoutValues(t *testing.T) {
	fs := newStore("web")
	live := &fakeLive{t: t, project: true, env: map[string]string{"REGION": "us-hidden-live", "FEATURE_SET": "declared-value-1"}}
	checker(t, fs, live).CheckAll(t.Context())
	d := fs.recorded["id-web"]
	if d.Status != store.StackDriftDrifted || len(d.Summary) != 1 {
		t.Fatalf("drift = %+v", d)
	}
	it := d.Summary[0]
	if it.Kind != "Project" || it.Name != "shop" || it.Action != ActionChanged || len(it.Fields) != 1 || !strings.Contains(it.Fields[0], "REGION") {
		t.Fatalf("item = %+v", it)
	}
	raw, _ := json.Marshal(d)
	for _, secret := range []string{"us-hidden-live", "declared-value-1"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("summary leaks %q: %s", secret, raw)
		}
	}
}

func TestCheckAll_ResourceDeletedLive(t *testing.T) {
	fs := newStore("web")
	checker(t, fs, &fakeLive{t: t}).CheckAll(t.Context())
	d := fs.recorded["id-web"]
	if d.Status != store.StackDriftDrifted || len(d.Summary) != 1 || d.Summary[0].Action != ActionMissing {
		t.Fatalf("drift = %+v", d)
	}
}

func TestCheckAll_LedgerOwnerMismatch(t *testing.T) {
	fs := newStore("web", "other")
	fs.owners["Project/shop"] = "id-other"
	live := &fakeLive{t: t, project: true, env: map[string]string{"REGION": "eu", "FEATURE_SET": "declared-value-1"}}
	checker(t, fs, live).CheckAll(t.Context())
	d := fs.recorded["id-web"]
	if d.Status != store.StackDriftDrifted || len(d.Summary) != 1 || d.Summary[0].Action != ActionOwnerChanged {
		t.Fatalf("drift = %+v", d)
	}
	if o := fs.recorded["id-other"]; o.Status != store.StackDriftNone {
		t.Fatalf("owning stack = %+v", o)
	}
}

func TestCheckAll_SkipsStackWithoutAppliedRevision(t *testing.T) {
	fs := newStore("web")
	fs.revs["id-web"] = fs.revs["id-web"][:1]
	checker(t, fs, &fakeLive{t: t}).CheckAll(t.Context())
	if len(fs.recorded) != 0 {
		t.Fatalf("recorded = %+v", fs.recorded)
	}
}

func TestCheckAll_ErrorsDoNotStopOtherStacks(t *testing.T) {
	fs := newStore("a", "b", "c")
	fs.panicOn = "id-a"
	fs.revs["id-b"][1].RenderedDocs = []byte("not json")
	live := &fakeLive{t: t}
	checker(t, fs, live).CheckAll(t.Context())
	if _, ok := fs.recorded["id-c"]; !ok || len(fs.recorded) != 1 {
		t.Fatalf("recorded = %+v", fs.recorded)
	}
	live.failAll = true
	delete(fs.recorded, "id-c")
	checker(t, fs, live).CheckAll(t.Context())
	if len(fs.recorded) != 0 {
		t.Fatalf("a failed live read recorded a status: %+v", fs.recorded)
	}
}

func TestRun_IntervalZeroDisabled(t *testing.T) {
	fs := newStore("web")
	done := make(chan struct{})
	go func() {
		checker(t, fs, &fakeLive{t: t}).Run(t.Context(), 0)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run with interval 0 did not return")
	}
	if len(fs.recorded) != 0 {
		t.Fatal("disabled loop checked a stack")
	}
}

func TestRun_ChecksThenStopsOnCancel(t *testing.T) {
	fs := newStore("web")
	ctx, cancel := context.WithCancel(t.Context())
	c := checker(t, fs, &fakeLive{t: t})
	done := make(chan struct{})
	go func() { c.Run(ctx, time.Hour); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for len(fs.recorded) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if len(fs.recorded) != 1 {
		t.Fatalf("recorded = %+v", fs.recorded)
	}
}

func TestIntervalFromEnv(t *testing.T) {
	for in, want := range map[string]time.Duration{"": DefaultInterval, "0": 0, "30s": 30 * time.Second, "junk": DefaultInterval, "-5m": DefaultInterval} {
		t.Setenv(EnvInterval, in)
		if got := IntervalFromEnv(); got != want {
			t.Errorf("%q = %v, want %v", in, got, want)
		}
	}
}

func TestSummarize_CapsItems(t *testing.T) {
	var p iac.Plan
	for _, n := range []string{"a", "b", "c"} {
		p.Changes = append(p.Changes, iac.Change{Kind: iac.KindProject, Name: n, Action: iac.ActionCreate})
	}
	if got := Summarize(p, 2); len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
}
