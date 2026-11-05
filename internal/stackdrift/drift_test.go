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
	ledger   []store.StackLedgerEntry
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
	return f.ledger, nil
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
	app     map[string]any
	// settings maps a runtime setting key to its live JSON body.
	settings map[string]map[string]any
	// taints maps a node name to its live taints; ids are "id-" plus the name.
	taints map[string][]map[string]string
}

func (l *fakeLive) nodeRead(path string) (any, bool) {
	if path == "/api/v1/nodes" {
		list := []any{}
		for name := range l.taints {
			list = append(list, map[string]string{"id": "id-" + name, "name": name})
		}
		return list, true
	}
	rest, ok := strings.CutPrefix(path, "/api/v1/nodes/id-")
	if !ok {
		return nil, false
	}
	name, sub, _ := strings.Cut(rest, "/")
	if sub == "labels" {
		return map[string]any{"labels": map[string]string{}}, true
	}
	return map[string]any{"taints": l.taints[name]}, true
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
	if nv, ok := l.nodeRead(path); ok && l.taints != nil {
		raw, _ := json.Marshal(nv)
		return json.Unmarshal(raw, out)
	}
	if key, ok := strings.CutPrefix(path, "/api/v1/settings/runtime/"); ok {
		body, found := l.settings[key]
		if !found {
			return &iac.StatusError{Status: 404, Message: "not found"}
		}
		raw, _ := json.Marshal(body)
		return json.Unmarshal(raw, out)
	}
	switch path {
	case "/api/v1/projects":
		v = []any{}
		if l.project {
			v = []any{map[string]string{"id": "p1", "name": "shop"}}
		}
	case "/api/v1/tags", "/api/v1/databases":
		v = []any{}
	case "/api/v1/projects/p1/environments":
		v = []any{}
	case "/api/v1/projects/p1/env":
		v = l.env
	case "/api/v1/apps/web":
		if l.app == nil {
			return &iac.StatusError{Status: 404, Message: "not found"}
		}
		v = l.app
	case "/api/v1/apps/web/secrets":
		v = []any{}
	case "/api/v1/apps/web/loadbalancer":
		return &iac.StatusError{Status: 404, Message: "the load-balancer feature is off by default; turn it on"}
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

func TestCheckAll_DigestPinnedLiveImageIsNotDrift(t *testing.T) {
	doc := "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  project: shop\n  service:\n    build: {type: image, image: \"nginx:1.26-alpine\"}\n    port: 80\n"
	fs := newStore("web")
	raw, _ := json.Marshal([]renderedFile{{Name: "stack.yaml", Content: doc}})
	fs.revs["id-web"][1].RenderedDocs = raw
	fs.ledger = []store.StackLedgerEntry{{StackID: "id-web", Kind: "App", Key: "App/web", LiveID: "web", SinceRevision: 1}}
	app := map[string]any{"name": "web", "image": "nginx:1.26-alpine@sha256:abc", "port": 80, "replicas": 1, "strategy": "blue-green", "bind_address": "private", "project_id": "p1", "tags": []string{"managed-by:web"}}
	checker(t, fs, &fakeLive{t: t, project: true, app: app}).CheckAll(t.Context())
	if d := fs.recorded["id-web"]; d.Status != store.StackDriftNone {
		t.Fatalf("drift = %+v", d)
	}
}

func installSettingStore(t *testing.T, value string) *fakeStore {
	t.Helper()
	doc := "version: 1\nkind: Setting\nmetadata: {name: " + "security.trust_loopback_forwarded_headers" + "}\nspec:\n  value: " + value + "\n"
	fs := newStore("base")
	raw, _ := json.Marshal([]renderedFile{{Name: "stack.yaml", Content: doc}})
	fs.stacks[0].ProjectID = ""
	fs.stacks[0].Scope = store.StackScopeInstall
	fs.revs["id-base"][1].RenderedDocs = raw
	fs.ledger = []store.StackLedgerEntry{{StackID: "id-base", Kind: "Setting", Key: "Setting/security.trust_loopback_forwarded_headers", LiveID: "security.trust_loopback_forwarded_headers", SinceRevision: 1}}
	return fs
}

func liveSettingBody(saved bool, pending bool) map[string]any {
	src := "saved"
	if !saved {
		src = "default"
	}
	return map[string]any{"key": "security.trust_loopback_forwarded_headers", "kind": "bool", "default": false, "value": false, "source": src, "apply": "restart", "pending_restart": pending}
}

func driftOf(t *testing.T, fs *fakeStore, body map[string]any) store.StackDrift {
	t.Helper()
	live := &fakeLive{t: t, settings: map[string]map[string]any{"security.trust_loopback_forwarded_headers": body}}
	c := checker(t, fs, live)
	c.ProjectName = func(context.Context, string) string { return "" }
	c.CheckAll(t.Context())
	return fs.recorded["id-base"]
}

func TestDriftSettingRestartNotDrift(t *testing.T) {
	fs := installSettingStore(t, "false")
	if d := driftOf(t, fs, liveSettingBody(true, true)); d.Status != store.StackDriftNone || len(d.Summary) != 0 {
		t.Fatalf("pending restart reported as drift: %+v", d)
	}
}

func TestDriftSettingChangedByHand(t *testing.T) {
	fs := installSettingStore(t, "true")
	d := driftOf(t, fs, liveSettingBody(true, false))
	if d.Status != store.StackDriftDrifted || len(d.Summary) != 1 || d.Summary[0].Kind != "Setting" || d.Summary[0].Action != ActionChanged {
		t.Fatalf("drift = %+v", d)
	}
	for _, f := range d.Summary[0].Fields {
		if strings.Contains(f, "true") || strings.Contains(f, "false") {
			t.Fatalf("summary leaks a value: %q", f)
		}
	}
}

func TestDriftSettingResetIsDrift(t *testing.T) {
	fs := installSettingStore(t, "false")
	d := driftOf(t, fs, liveSettingBody(false, false))
	if d.Status != store.StackDriftDrifted || d.Summary[0].Fields[0] != "source" {
		t.Fatalf("drift = %+v", d)
	}
}

func installNodeDrift(t *testing.T, live []map[string]string) store.StackDrift {
	t.Helper()
	doc := "version: 1\nkind: Node\nmetadata:\n  name: worker-1\nspec:\n  taints:\n    - {key: gpu, value: \"a\", effect: NoSchedule}\n"
	fs := newStore("base")
	raw, _ := json.Marshal([]renderedFile{{Name: "stack.yaml", Content: doc}})
	fs.stacks[0].ProjectID, fs.stacks[0].Scope = "", store.StackScopeInstall
	fs.revs["id-base"][1].RenderedDocs = raw
	fs.ledger = []store.StackLedgerEntry{{StackID: "id-base", Kind: "Node", Key: "Node/worker-1", LiveID: "id-worker-1", SinceRevision: 1}}
	c := checker(t, fs, &fakeLive{t: t, taints: map[string][]map[string]string{"worker-1": live}})
	c.ProjectName = func(context.Context, string) string { return "" }
	c.CheckAll(t.Context())
	return fs.recorded["id-base"]
}

func TestDriftNodeTaintChangedByHand(t *testing.T) {
	declared := map[string]string{"key": "gpu", "value": "a", "effect": "NoSchedule"}
	if d := installNodeDrift(t, []map[string]string{declared}); d.Status != store.StackDriftNone || d.CheckedAt.IsZero() {
		t.Fatalf("matching taints = %+v", d)
	}
	d := installNodeDrift(t, []map[string]string{declared, {"key": "lab-hand", "value": "secret-ish", "effect": "NoSchedule"}})
	if d.Status != store.StackDriftDrifted || len(d.Summary) != 1 || d.Summary[0].Kind != "Node" || d.Summary[0].Action != ActionChanged || len(d.Summary[0].Fields) == 0 {
		t.Fatalf("hand taint = %+v", d)
	}
	raw, _ := json.Marshal(d.Summary)
	if strings.Contains(string(raw), "secret-ish") {
		t.Fatalf("summary leaks a value: %s", raw)
	}
}
