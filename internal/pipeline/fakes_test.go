package pipeline

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// execResult is what a fake exec returns for a script.
type execResult struct {
	Output string
	Exit   int
	// Block makes the exec hang until its context is cancelled.
	Block bool
}

type fakeRuntime struct {
	mu         sync.Mutex
	seq        int
	containers map[string]docker.ContainerState
	created    []docker.ContainerSpec
	removed    []string
	volumes    []string
	scripts    []string
	handler    func(script string) execResult
	failCreate error
}

func newFakeRuntime(h func(string) execResult) *fakeRuntime {
	return &fakeRuntime{containers: map[string]docker.ContainerState{}, handler: h}
}

func (f *fakeRuntime) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failCreate != nil {
		return "", f.failCreate
	}
	f.seq++
	id := fmt.Sprintf("c%d", f.seq)
	f.containers[id] = docker.ContainerState{ID: id, Name: spec.Name, Image: spec.Image}
	f.created = append(f.created, spec)
	return id, nil
}

func (f *fakeRuntime) Start(context.Context, string) error { return nil }

func (f *fakeRuntime) Remove(_ context.Context, id string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.containers, id)
	f.removed = append(f.removed, id)
	return nil
}

func (f *fakeRuntime) ListByPrefix(_ context.Context, prefix string) ([]docker.ContainerState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []docker.ContainerState
	for _, c := range f.containers {
		if strings.HasPrefix(c.Name, prefix) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeRuntime) EnsureVolume(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.volumes = append(f.volumes, name)
	return nil
}

func (f *fakeRuntime) ExecWithInput(ctx context.Context, _ string, _ []string, stdin io.Reader) (io.ReadCloser, error) {
	b, _ := io.ReadAll(stdin)
	script := string(b)
	f.mu.Lock()
	f.scripts = append(f.scripts, script)
	f.mu.Unlock()
	res := f.handler(script)
	if res.Block {
		return &blockingReader{ctx: ctx}, nil
	}
	return &scriptReader{r: strings.NewReader(res.Output), exit: res.Exit}, nil
}

func (f *fakeRuntime) liveContainers() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.containers)
}

func (f *fakeRuntime) ran(substr string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, s := range f.scripts {
		if strings.Contains(s, substr) {
			n++
		}
	}
	return n
}

type scriptReader struct {
	r    *strings.Reader
	exit int
}

func (s *scriptReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if err == io.EOF && s.exit != 0 {
		return n, &docker.ExecExitError{ExitCode: s.exit}
	}
	return n, err
}
func (s *scriptReader) Close() error { return nil }

type blockingReader struct {
	ctx    context.Context
	closed chan struct{}
	init   sync.Once
	shut   sync.Once
}

func (b *blockingReader) ch() chan struct{} {
	b.init.Do(func() { b.closed = make(chan struct{}) })
	return b.closed
}

func (b *blockingReader) Read([]byte) (int, error) {
	select {
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	case <-b.ch():
		return 0, io.ErrClosedPipe
	}
}

func (b *blockingReader) Close() error {
	b.shut.Do(func() { close(b.ch()) })
	return nil
}

type fakeActions struct {
	mu         sync.Mutex
	builds     []BuildRequest
	deploys    []DeployRequest
	promotes   [][2]string
	rollback   []string
	notifies   []string
	failDeploy error
}

func (a *fakeActions) Build(_ context.Context, req BuildRequest, log func(string)) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.builds = append(a.builds, req)
	log("building")
	return req.Image + ":" + req.Tag, nil
}

func (a *fakeActions) Deploy(_ context.Context, req DeployRequest, _ func(string)) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.deploys = append(a.deploys, req)
	return a.failDeploy
}

func (a *fakeActions) Promote(_ context.Context, from, to string, _ func(string)) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.promotes = append(a.promotes, [2]string{from, to})
	return "img:promoted", nil
}

func (a *fakeActions) Rollback(_ context.Context, svc string, _ func(string)) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rollback = append(a.rollback, svc)
	return "img:prev", nil
}

func (a *fakeActions) Notify(_ context.Context, app string, ok bool, msg string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.notifies = append(a.notifies, fmt.Sprintf("%s|%v|%s", app, ok, msg))
	return nil
}

type fakeSecrets map[string]string

func (f fakeSecrets) Resolve(_ context.Context, _, key string) (string, error) {
	if v, ok := f[key]; ok {
		return v, nil
	}
	return "", fmt.Errorf("no secret %s", key)
}

type harness struct {
	t   *testing.T
	db  *store.DB
	rt  *fakeRuntime
	act *fakeActions
	e   *Engine
	seq int
	mu  sync.Mutex
}

func newHarness(t *testing.T, h func(string) execResult) *harness {
	t.Helper()
	hs := &harness{t: t, db: openStore(t), rt: newFakeRuntime(h), act: &fakeActions{}}
	hs.e = hs.newEngine()
	t.Cleanup(func() { hs.e.Close() })
	return hs
}

func (h *harness) newEngine() *Engine {
	return New(Config{
		Store: h.db, Actions: h.act, Secrets: fakeSecrets{"TOKEN": "s3cr3t-value"}, NamePrefix: "t",
		Runtime: func(string) (Runtime, error) { return h.rt, nil },
		NewID: func() string {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.seq++
			return fmt.Sprintf("id%03d", h.seq)
		},
	})
}

func (h *harness) save(yamlText string) store.Pipeline {
	h.t.Helper()
	now := time.Now().UTC()
	p, err := h.db.SavePipeline(context.Background(), store.Pipeline{ID: "pipe-" + fmt.Sprint(now.UnixNano()), AppName: "web", Name: "ci", YAML: yamlText, Enabled: true, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		h.t.Fatalf("save pipeline: %v", err)
	}
	return p
}

func (h *harness) start(p store.Pipeline, opt StartOptions) store.PipelineRun {
	h.t.Helper()
	if opt.Trigger == "" {
		opt.Trigger = TriggerManual
	}
	r, err := h.e.Start(context.Background(), p, opt)
	if err != nil {
		h.t.Fatalf("start: %v", err)
	}
	return r
}

// until ticks the engine until pred holds for the run or the deadline passes.
func (h *harness) until(runID string, pred func(store.PipelineRun) bool) store.PipelineRun {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if err := h.e.Tick(context.Background()); err != nil {
			h.t.Fatalf("tick: %v", err)
		}
		r, err := h.db.GetPipelineRun(context.Background(), runID)
		if err != nil {
			h.t.Fatal(err)
		}
		if pred(r) {
			return r
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("timed out; run status=%s reason=%q\njobs=%s", r.Status, r.Reason, h.dump(runID))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (h *harness) done(runID string) store.PipelineRun {
	h.t.Helper()
	return h.until(runID, func(r store.PipelineRun) bool { return store.IsPipelineTerminal(r.Status) })
}

func (h *harness) dump(runID string) string {
	jobs, _ := h.db.ListPipelineJobs(context.Background(), runID)
	var sb strings.Builder
	for _, j := range jobs {
		fmt.Fprintf(&sb, "%s=%s(%s) ", j.Key, j.Status, j.Reason)
		for _, s := range j.Steps {
			fmt.Fprintf(&sb, "[%d %s %s %q] ", s.Index, s.Kind, s.Status, s.Reason)
		}
	}
	return sb.String()
}

func (h *harness) jobs(runID string) map[string]store.PipelineJob {
	h.t.Helper()
	list, err := h.db.ListPipelineJobs(context.Background(), runID)
	if err != nil {
		h.t.Fatal(err)
	}
	m := map[string]store.PipelineJob{}
	for _, j := range list {
		m[j.Key] = j
	}
	return m
}

func (h *harness) logs(runID string) string {
	h.t.Helper()
	lines, err := h.db.ListPipelineLogs(context.Background(), runID, "", 0, 10000)
	if err != nil {
		h.t.Fatal(err)
	}
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(l.Line + "\n")
	}
	return sb.String()
}

func openStore(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), t.TempDir()+"/t.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
