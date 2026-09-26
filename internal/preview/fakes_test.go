package preview

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type memStore struct {
	mu       sync.Mutex
	settings map[string]AppSettings
	records  map[string]Record
	state    map[string]string
}

func newMemStore() *memStore {
	return &memStore{settings: map[string]AppSettings{}, records: map[string]Record{}, state: map[string]string{}}
}

func (s *memStore) GetPreviewSettings(_ context.Context, app string) (AppSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings[app], nil
}

func (s *memStore) SavePreviewSettings(_ context.Context, a AppSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings[a.App] = a
	return nil
}

func (s *memStore) DeletePreviewSettings(_ context.Context, app string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.settings, app)
	return nil
}

func (s *memStore) UpsertPreviewRecord(_ context.Context, r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[r.DeploymentID] = r
	return nil
}

func (s *memStore) GetPreviewRecord(_ context.Context, id string) (*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[id]
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (s *memStore) list(app string) []Record {
	var out []Record
	for _, r := range s.records {
		if app == "" || r.App == app {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CapturedAt.After(out[j].CapturedAt) })
	return out
}

func (s *memStore) ListPreviewRecords(_ context.Context, app string) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list(app), nil
}

func (s *memStore) ListAllPreviewRecords(_ context.Context) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list(""), nil
}

func (s *memStore) DeletePreviewRecords(_ context.Context, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		delete(s.records, id)
	}
	return nil
}

func (s *memStore) TouchPreviewViewed(_ context.Context, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.records[id]
	r.LastViewedAt = at
	s.records[id] = r
	return nil
}

func (s *memStore) GetPreviewState(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state[key], nil
}

func (s *memStore) SetPreviewState(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state[key] = value
	return nil
}

type fakeResolver struct {
	mu      sync.Mutex
	target  Target
	err     error
	current string
	gone    map[string]bool
	calls   int
}

func (r *fakeResolver) Resolve(context.Context, string, string) (Target, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return r.target, r.err
}

func (r *fakeResolver) CurrentDeployment(context.Context, string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.current, nil
}

func (r *fakeResolver) AppExists(_ context.Context, app string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.gone[app], nil
}

type fakeBrowser struct {
	r      *fakeRunner
	closed bool
}

func (b *fakeBrowser) Endpoint() string { return "127.0.0.1:1" }
func (b *fakeBrowser) Close() {
	b.r.mu.Lock()
	defer b.r.mu.Unlock()
	b.r.closed++
}

type fakeRunner struct {
	mu          sync.Mutex
	imageID     string
	pulled      bool
	ensureErr   error
	startErr    error
	specs       []docker.BrowserSpec
	started     int
	closed      int
	containers  []docker.LabeledContainer
	removed     []string
	imageInUse  bool
	removedImgs []string
}

func (r *fakeRunner) EnsureImageID(context.Context, string) (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.imageID, r.pulled, r.ensureErr
}

func (r *fakeRunner) StartBrowser(_ context.Context, spec docker.BrowserSpec) (Browser, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.startErr != nil {
		return nil, r.startErr
	}
	r.specs = append(r.specs, spec)
	r.started++
	return &fakeBrowser{r: r}, nil
}

func (r *fakeRunner) ListContainersByLabel(context.Context, string) ([]docker.LabeledContainer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]docker.LabeledContainer(nil), r.containers...), nil
}

func (r *fakeRunner) Remove(_ context.Context, id string, _ bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.removed = append(r.removed, id)
	return nil
}

func (r *fakeRunner) ImageInUse(context.Context, string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.imageInUse, nil
}

func (r *fakeRunner) RemoveImageByID(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.removedImgs = append(r.removedImgs, id)
	return nil
}

type fakeShooter struct {
	mu     sync.Mutex
	result *ShotResult
	err    error
	calls  int
	block  chan struct{}
	active int
	maxAct int
	reqs   []ShotRequest
}

func (f *fakeShooter) Shoot(ctx context.Context, _ string, req ShotRequest) (*ShotResult, error) {
	f.mu.Lock()
	f.calls++
	f.active++
	f.maxAct = max(f.maxAct, f.active)
	f.reqs = append(f.reqs, req)
	block := f.block
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
		}
	}
	f.mu.Lock()
	f.active--
	f.mu.Unlock()
	return f.result, f.err
}

func testPNG(t *testing.T, paint func(x, y int) color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1280, 800))
	for y := 0; y < 800; y++ {
		for x := 0; x < 1280; x++ {
			img.Set(x, y, paint(x, y))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

var (
	stripedOnce, flatOnce sync.Once
	stripedData, flatData []byte
)

func stripedPNG(t *testing.T) []byte {
	stripedOnce.Do(func() {
		stripedData = testPNG(t, func(x, y int) color.Color {
			if (x/40+y/40)%2 == 0 {
				return color.RGBA{R: 20, G: 40, B: 200, A: 255}
			}
			return color.RGBA{R: 250, G: 250, B: 240, A: 255}
		})
	})
	return stripedData
}

func flatPNG(t *testing.T) []byte {
	flatOnce.Do(func() {
		flatData = testPNG(t, func(int, int) color.Color { return color.White })
	})
	return flatData
}

type harness struct {
	m      *Manager
	store  *memStore
	res    *fakeResolver
	run    *fakeRunner
	shoot  *fakeShooter
	dir    string
	ram    int64
	disk   int64
	nowMu  sync.Mutex
	nowVal time.Time
}

func newHarness(t *testing.T, mutate func(*Config)) *harness {
	t.Helper()
	h := &harness{
		store: newMemStore(),
		res:   &fakeResolver{target: Target{DeploymentID: "dep_1", Image: "img:1", Network: "ns-app-web", Host: "web", Port: 3000}, current: "dep_1", gone: map[string]bool{}},
		run:   &fakeRunner{imageID: "sha256:browser", pulled: true},
		dir:   t.TempDir(),
		ram:   4 << 30,
		disk:  50 << 30,
	}
	h.nowVal = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	h.shoot = &fakeShooter{result: &ShotResult{HTTPStatus: 200, FinalURL: "http://web:3000/", PNG: stripedPNG(t)}}
	cfg := ConfigFromEnv(func(k string) (string, bool) {
		if k == EnvEnabled {
			return "true", true
		}
		return "", false
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if mutate != nil {
		mutate(&cfg)
	}
	h.m = New(cfg, Deps{Store: h.store, Resolver: h.res, Runner: h.run, DataDir: h.dir, Namespace: "ns", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))},
		WithShooter(h.shoot),
		WithClock(func() time.Time { h.nowMu.Lock(); defer h.nowMu.Unlock(); return h.nowVal }),
		WithHostProbes(func() (int64, int64, error) { return 8 << 30, h.ram, nil }, func(string) (int64, error) { return h.disk, nil }),
	)
	return h
}

func (h *harness) advance(d time.Duration) {
	h.nowMu.Lock()
	defer h.nowMu.Unlock()
	h.nowVal = h.nowVal.Add(d)
}

func (h *harness) enable(t *testing.T, app string) {
	t.Helper()
	on := true
	if _, err := h.m.SaveSettings(context.Background(), app, SettingsPatch{Enabled: &on}); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) waitIdle(t *testing.T, app string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for h.m.Capturing(app) {
		if time.Now().After(deadline) {
			t.Fatal("capture did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	for h.m.busy() {
		if time.Now().After(deadline) {
			t.Fatal("worker did not go idle")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (h *harness) record(id string) (Record, bool) {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	r, ok := h.store.records[id]
	return r, ok
}

var errBoom = errors.New("boom")
