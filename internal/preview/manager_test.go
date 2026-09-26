package preview

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestNotifyReady_CapturesOptedInAppAndStoresThumbnail(t *testing.T) {
	h := newHarness(t, nil)
	h.enable(t, "web")
	h.m.NotifyReady("web", "img:1")
	h.waitIdle(t, "web")

	r, ok := h.record("dep_1")
	if !ok || r.Status != StatusOK {
		t.Fatalf("record = %+v ok=%v, want ok status", r, ok)
	}
	if r.Width != 640 || r.Height != 400 || r.Bytes == 0 || r.HTTPStatus != 200 {
		t.Errorf("record fields = %+v", r)
	}
	p, err := h.m.fs.Path("web", "dep_1")
	if err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(p); err != nil || st.Size() != r.Bytes {
		t.Errorf("thumbnail file stat = %v err = %v, want %d bytes", st, err, r.Bytes)
	}
	if h.run.closed != 1 || h.run.started != 1 {
		t.Errorf("browser started=%d closed=%d, want 1/1", h.run.started, h.run.closed)
	}
}

func TestNotifyReady_SkipsWhenNotOptedInOrDisabled(t *testing.T) {
	t.Run("app not opted in", func(t *testing.T) {
		h := newHarness(t, nil)
		h.m.NotifyReady("web", "img:1")
		h.waitIdle(t, "web")
		if h.run.started != 0 || len(h.store.records) != 0 {
			t.Errorf("started=%d records=%d, want none", h.run.started, len(h.store.records))
		}
	})
	t.Run("kill switch off", func(t *testing.T) {
		h := newHarness(t, func(c *Config) { c.Enabled = false })
		h.enable(t, "web")
		h.m.NotifyReady("web", "img:1")
		h.waitIdle(t, "web")
		if h.res.calls != 0 || h.run.started != 0 {
			t.Errorf("resolver calls=%d started=%d, want none", h.res.calls, h.run.started)
		}
		if err := h.m.Capture(context.Background(), "web"); err == nil {
			t.Error("manual capture must be refused while the kill switch is off")
		}
	})
}

func TestNotifyReady_DedupesSameRelease(t *testing.T) {
	h := newHarness(t, nil)
	h.enable(t, "web")
	for i := 0; i < 5; i++ {
		h.m.NotifyReady("web", "img:1")
		h.waitIdle(t, "web")
	}
	if h.shoot.calls != 1 {
		t.Errorf("shoot calls = %d, want 1", h.shoot.calls)
	}
	h.m.NotifyReady("web", "img:2")
	h.waitIdle(t, "web")
	if h.res.calls != 2 {
		t.Errorf("resolver calls = %d, want 2 (one per release)", h.res.calls)
	}
}

func TestProcess_DoesNotRecaptureExistingDeployment(t *testing.T) {
	h := newHarness(t, nil)
	h.enable(t, "web")
	h.store.records["dep_1"] = Record{DeploymentID: "dep_1", App: "web", Status: StatusSkipped, Reason: ReasonHTTPStatus, CapturedAt: h.nowVal}
	h.m.NotifyReady("web", "img:1")
	h.waitIdle(t, "web")
	if h.shoot.calls != 0 {
		t.Errorf("shoot calls = %d, want 0", h.shoot.calls)
	}
}

func TestCapture_SkipRules(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(h *harness)
		wantStatus string
		wantReason string
		wantStart  int
	}{
		{"low free ram", func(h *harness) { h.ram = 512 << 20 }, StatusSkipped, ReasonLowRAM, 0},
		{"exactly at the ram floor passes", func(h *harness) { h.ram = 768 << 20 }, StatusOK, "", 1},
		{"low free disk", func(h *harness) { h.disk = 1 << 30 }, StatusSkipped, ReasonLowDisk, 0},
		{"browser image unavailable", func(h *harness) { h.run.ensureErr = errBoom }, StatusSkipped, ReasonImageUnusable, 0},
		{"browser cannot start", func(h *harness) { h.run.startErr = errBoom }, StatusFailed, ReasonCaptureFailed, 0},
		{"app answers 404", func(h *harness) { h.shoot.result = &ShotResult{HTTPStatus: 404} }, StatusSkipped, ReasonHTTPStatus, 1},
		{"app answers 401", func(h *harness) { h.shoot.result = &ShotResult{HTTPStatus: 401} }, StatusSkipped, ReasonAuthWall, 1},
		{"login redirect", func(h *harness) {
			h.shoot.result = &ShotResult{HTTPStatus: 200, FinalURL: "http://web:3000/login", PNG: stripedPNG(t)}
		}, StatusSkipped, ReasonAuthWall, 1},
		{"blank page", func(h *harness) {
			h.shoot.result = &ShotResult{HTTPStatus: 200, FinalURL: "http://web:3000/", PNG: flatPNG(t)}
		}, StatusSkipped, ReasonBlankImage, 1},
		{"garbage image", func(h *harness) {
			h.shoot.result = &ShotResult{HTTPStatus: 200, FinalURL: "http://web:3000/", PNG: []byte("garbage")}
		}, StatusFailed, ReasonBadImage, 1},
		{"browser error", func(h *harness) { h.shoot.err = errBoom }, StatusFailed, ReasonCaptureFailed, 1},
		{"target unreachable", func(h *harness) { h.shoot.result = &ShotResult{} }, StatusSkipped, ReasonUnreachable, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, nil)
			h.enable(t, "web")
			tt.setup(h)
			h.m.NotifyReady("web", "img:1")
			h.waitIdle(t, "web")
			r, ok := h.record("dep_1")
			if !ok {
				t.Fatal("no record stored")
			}
			if r.Status != tt.wantStatus || r.Reason != tt.wantReason {
				t.Errorf("status/reason = %s/%s, want %s/%s", r.Status, r.Reason, tt.wantStatus, tt.wantReason)
			}
			if h.run.started != tt.wantStart {
				t.Errorf("browser started = %d, want %d", h.run.started, tt.wantStart)
			}
			if h.run.started != h.run.closed {
				t.Errorf("browser leaked: started %d closed %d", h.run.started, h.run.closed)
			}
			if tt.wantStatus != StatusOK && h.m.fs.Exists("web", "dep_1") {
				t.Error("a rejected capture must not leave a thumbnail file")
			}
		})
	}
}

func TestCapture_ResolverSkipIsRecorded(t *testing.T) {
	h := newHarness(t, nil)
	h.enable(t, "web")
	h.res.err = &SkipError{DeploymentID: "dep_9", Reason: ReasonNoAppNetwork, Detail: "no network"}
	h.m.NotifyReady("web", "img:1")
	h.waitIdle(t, "web")
	r, ok := h.record("dep_9")
	if !ok || r.Status != StatusSkipped || r.Reason != ReasonNoAppNetwork {
		t.Errorf("record = %+v ok=%v", r, ok)
	}
	if h.run.started != 0 {
		t.Errorf("browser started = %d, want 0", h.run.started)
	}
}

func TestCapture_SupersededResolverSkipRecordsNothing(t *testing.T) {
	h := newHarness(t, nil)
	h.enable(t, "web")
	h.res.err = &SkipError{Reason: "superseded"}
	h.m.NotifyReady("web", "img:1")
	h.waitIdle(t, "web")
	if len(h.store.records) != 0 {
		t.Errorf("records = %v, want none", h.store.records)
	}
}

func TestCapture_ContainerSpecIsLockedDown(t *testing.T) {
	h := newHarness(t, nil)
	h.enable(t, "web")
	h.m.NotifyReady("web", "img:1")
	h.waitIdle(t, "web")
	if len(h.run.specs) != 1 {
		t.Fatalf("specs = %d", len(h.run.specs))
	}
	s := h.run.specs[0]
	if s.Network != "ns-app-web" {
		t.Errorf("network = %q, want the app network only", s.Network)
	}
	if s.MemoryBytes != 512<<20 || s.NanoCPUs != 1_000_000_000 {
		t.Errorf("limits = %d bytes, %d nanocpus", s.MemoryBytes, s.NanoCPUs)
	}
	if s.User == "" || s.User == "root" || s.PidsLimit == 0 || s.Tmpfs["/tmp"] == "" {
		t.Errorf("spec is not locked down: %+v", s)
	}
	if s.Labels["ns"+roleLabelSuffix] != roleValue || s.Labels["ns"+deploymentLabelSuffix] != "dep_1" {
		t.Errorf("labels = %v", s.Labels)
	}
	if !strings.HasPrefix(s.Name, "ns-preview-") || s.Image != h.m.Config().Image {
		t.Errorf("name/image = %q %q", s.Name, s.Image)
	}
	req := h.shoot.reqs[0]
	if req.URL != "http://web:3000/" || req.Width != 1280 || req.Height != 800 {
		t.Errorf("shot request = %+v", req)
	}
}

func TestQueue_SingleFlightAndCoalescing(t *testing.T) {
	h := newHarness(t, nil)
	for _, app := range []string{"a", "b", "c"} {
		h.enable(t, app)
	}
	h.shoot.block = make(chan struct{})
	h.m.NotifyReady("a", "img:1")
	deadline := time.Now().Add(2 * time.Second)
	for {
		h.shoot.mu.Lock()
		n := h.shoot.calls
		h.shoot.mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first capture never started")
		}
		time.Sleep(2 * time.Millisecond)
	}
	h.m.NotifyReady("b", "img:1")
	h.m.NotifyReady("b", "img:2")
	h.m.NotifyReady("c", "img:1")
	h.m.mu.Lock()
	queued := len(h.m.queue)
	h.m.mu.Unlock()
	if queued != 2 {
		t.Errorf("queued = %d, want 2 (b coalesced, c waiting)", queued)
	}
	close(h.shoot.block)
	h.waitIdle(t, "a")
	if h.shoot.maxAct != 1 {
		t.Errorf("max concurrent captures = %d, want 1", h.shoot.maxAct)
	}
}

func TestQueue_DropsOldestWhenFull(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.QueueDepth = 2 })
	h.m.mu.Lock()
	h.m.running = true
	h.m.mu.Unlock()
	h.m.enqueue(job{app: "a"})
	h.m.enqueue(job{app: "b"})
	h.m.enqueue(job{app: "c"})
	h.m.mu.Lock()
	defer h.m.mu.Unlock()
	if len(h.m.queue) != 2 || h.m.queue[0].app != "b" || h.m.queue[1].app != "c" {
		t.Errorf("queue = %+v, want [b c]", h.m.queue)
	}
}

func TestManualCapture_FailureKeepsExistingThumbnail(t *testing.T) {
	h := newHarness(t, nil)
	h.enable(t, "web")
	if err := h.m.Capture(context.Background(), "web"); err != nil {
		t.Fatal(err)
	}
	h.waitIdle(t, "web")
	first, _ := h.record("dep_1")
	if first.Status != StatusOK {
		t.Fatalf("first capture = %+v", first)
	}
	h.advance(time.Minute)
	h.shoot.result = &ShotResult{HTTPStatus: 500}
	if err := h.m.Capture(context.Background(), "web"); err != nil {
		t.Fatal(err)
	}
	h.waitIdle(t, "web")
	after, _ := h.record("dep_1")
	if after.Status != StatusOK || !after.CapturedAt.Equal(first.CapturedAt) {
		t.Errorf("record after failed recapture = %+v, want the original ok record", after)
	}
	if !h.m.fs.Exists("web", "dep_1") {
		t.Error("thumbnail file must survive a failed recapture")
	}
}

func TestManualCapture_RequiresAppOptIn(t *testing.T) {
	h := newHarness(t, nil)
	if err := h.m.Capture(context.Background(), "web"); err == nil {
		t.Error("capture for an app that has not opted in must fail")
	}
}

func TestEnsureImage_TracksOnlyImagesItPulled(t *testing.T) {
	t.Run("pulled by us is tracked", func(t *testing.T) {
		h := newHarness(t, nil)
		h.enable(t, "web")
		h.m.NotifyReady("web", "img:1")
		h.waitIdle(t, "web")
		if st := h.m.BrowserImage(context.Background()); st.ID != "sha256:browser" || st.LastUsed.IsZero() {
			t.Errorf("tracked image = %+v", st)
		}
	})
	t.Run("pre-existing image is never tracked", func(t *testing.T) {
		h := newHarness(t, nil)
		h.run.pulled = false
		h.enable(t, "web")
		h.m.NotifyReady("web", "img:1")
		h.waitIdle(t, "web")
		if st := h.m.BrowserImage(context.Background()); st.ID != "" {
			t.Errorf("image the user pulled must not be tracked, got %+v", st)
		}
	})
}

func TestSweep_RemovesOrphanCaptureContainers(t *testing.T) {
	h := newHarness(t, nil)
	now := h.nowVal
	h.run.containers = []docker.LabeledContainer{
		{ID: "old", Name: "ns-preview-old", Created: now.Add(-10 * time.Minute)},
		{ID: "fresh", Name: "ns-preview-fresh", Created: now.Add(-20 * time.Second)},
		{ID: "edge", Name: "ns-preview-edge", Created: now.Add(-61 * time.Second)},
	}
	h.m.Sweep(context.Background())
	got := strings.Join(h.run.removed, ",")
	if got != "old,edge" {
		t.Errorf("removed = %q, want old,edge (fresh one may still be running)", got)
	}
}

func TestSweep_OrphanFilesAndDanglingRows(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	if err := h.m.fs.Write("web", "dep_keep", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := h.m.fs.Write("web", "dep_orphan", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := h.m.fs.Write("ghost", "dep_ghost", []byte("x")); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(h.dir, "previews", "web", ".tmp-123")
	if err := os.WriteFile(stray, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.store.records["dep_keep"] = Record{DeploymentID: "dep_keep", App: "web", Status: StatusOK, Bytes: 1, CapturedAt: h.nowVal}
	h.store.records["dep_dangling"] = Record{DeploymentID: "dep_dangling", App: "web", Status: StatusOK, Bytes: 1, CapturedAt: h.nowVal}
	h.store.records["dep_skipped"] = Record{DeploymentID: "dep_skipped", App: "web", Status: StatusSkipped, CapturedAt: h.nowVal}
	h.res.current = "dep_keep"

	h.m.Sweep(ctx)

	if !h.m.fs.Exists("web", "dep_keep") {
		t.Error("file with a row was removed")
	}
	if h.m.fs.Exists("web", "dep_orphan") || h.m.fs.Exists("ghost", "dep_ghost") {
		t.Error("orphan files were kept")
	}
	if _, err := os.Stat(stray); err == nil {
		t.Error("stray temp file was kept")
	}
	if _, ok := h.record("dep_dangling"); ok {
		t.Error("row whose file is gone was kept")
	}
	if _, ok := h.record("dep_skipped"); !ok {
		t.Error("skipped row (no file by design) was removed")
	}
}

func TestSweep_RemovesPreviewsOfDeletedApps(t *testing.T) {
	h := newHarness(t, nil)
	if err := h.m.fs.Write("gone", "dep_g", []byte("x")); err != nil {
		t.Fatal(err)
	}
	h.store.records["dep_g"] = Record{DeploymentID: "dep_g", App: "gone", Status: StatusOK, Bytes: 1, CapturedAt: h.nowVal}
	h.res.gone["gone"] = true
	h.m.Sweep(context.Background())
	if _, ok := h.record("dep_g"); ok || h.m.fs.Exists("gone", "dep_g") {
		t.Error("previews of a deleted app must be removed by the sweep")
	}
}

func TestSweep_AppliesRetentionAndProtectsProduction(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.KeepPerApp = 2 })
	for i := 0; i < 5; i++ {
		id := "dep_" + string(rune('a'+i))
		if err := h.m.fs.Write("web", id, []byte("x")); err != nil {
			t.Fatal(err)
		}
		h.store.records[id] = Record{DeploymentID: id, App: "web", Status: StatusOK, Bytes: 1, CapturedAt: h.nowVal.Add(-time.Duration(i) * time.Hour)}
	}
	h.res.current = "dep_e"
	h.m.Sweep(context.Background())
	var kept []string
	for id := range h.store.records {
		kept = append(kept, id)
	}
	if len(kept) != 3 {
		t.Errorf("kept %v, want 2 newest plus the production one", kept)
	}
	if _, ok := h.record("dep_e"); !ok {
		t.Error("current production preview was evicted")
	}
}

func TestPruneImage(t *testing.T) {
	ctx := context.Background()
	track := func(h *harness, lastUsed time.Time) {
		_ = h.store.SetPreviewState(ctx, stateImageID, "sha256:browser")
		_ = h.store.SetPreviewState(ctx, stateImageRef, "browser:1")
		_ = h.store.SetPreviewState(ctx, stateImageUsedAt, lastUsed.Format(time.RFC3339))
	}
	tests := []struct {
		name    string
		setup   func(h *harness)
		removed int
	}{
		{"idle past ttl is removed", func(h *harness) { track(h, h.nowVal.Add(-15*24*time.Hour)) }, 1},
		{"recently used is kept", func(h *harness) { track(h, h.nowVal.Add(-13*24*time.Hour)) }, 0},
		{"idle but in use is kept", func(h *harness) { track(h, h.nowVal.Add(-30*24*time.Hour)); h.run.imageInUse = true }, 0},
		{"feature disabled removes even a fresh image", func(h *harness) {
			track(h, h.nowVal.Add(-time.Hour))
			h.m.cfg.Enabled = false
		}, 1},
		{"untracked image is never removed", func(*harness) {}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, nil)
			tt.setup(h)
			h.m.Sweep(ctx)
			if len(h.run.removedImgs) != tt.removed {
				t.Errorf("images removed = %v, want %d", h.run.removedImgs, tt.removed)
			}
			if tt.removed == 1 && h.m.BrowserImage(ctx).ID != "" {
				t.Error("tracking state must be cleared after removal")
			}
		})
	}
}

func TestDeleteAppAndPrune(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	for _, id := range []string{"dep_a", "dep_b"} {
		if err := h.m.fs.Write("web", id, []byte("x")); err != nil {
			t.Fatal(err)
		}
		h.store.records[id] = Record{DeploymentID: id, App: "web", Status: StatusOK, Bytes: 5, CapturedAt: h.nowVal}
	}
	h.res.current = "dep_a"
	res, err := h.m.Prune(ctx, "web", false)
	if err != nil || res.Removed != 0 {
		t.Errorf("default prune = %+v err %v, want nothing removed under the limits", res, err)
	}
	res, err = h.m.Prune(ctx, "web", true)
	if err != nil || res.Removed != 2 || res.FreedBytes != 10 {
		t.Errorf("prune all = %+v err %v", res, err)
	}
	if h.m.fs.Exists("web", "dep_a") || len(h.store.records) != 0 {
		t.Error("prune --all left previews behind")
	}
	if err := h.m.fs.Write("web", "dep_c", []byte("x")); err != nil {
		t.Fatal(err)
	}
	h.store.records["dep_c"] = Record{DeploymentID: "dep_c", App: "web", Status: StatusOK, CapturedAt: h.nowVal}
	if err := h.m.DeleteApp(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	if h.m.fs.Exists("web", "dep_c") || len(h.store.records) != 0 {
		t.Error("DeleteApp left previews behind")
	}
}

func TestOpenImage(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	if err := h.m.fs.Write("web", "dep_a", []byte("x")); err != nil {
		t.Fatal(err)
	}
	h.store.records["dep_a"] = Record{DeploymentID: "dep_a", App: "web", Status: StatusOK, CapturedAt: h.nowVal.Add(-time.Hour)}
	h.store.records["dep_skip"] = Record{DeploymentID: "dep_skip", App: "web", Status: StatusSkipped, CapturedAt: h.nowVal}
	if _, _, err := h.m.OpenImage(ctx, "web", "dep_a"); err != nil {
		t.Errorf("open ok image: %v", err)
	}
	if r, _ := h.record("dep_a"); r.LastViewedAt.IsZero() {
		t.Error("serving an image must stamp last_viewed_at")
	}
	for _, tc := range []struct{ app, id string }{{"other", "dep_a"}, {"web", "dep_skip"}, {"web", "dep_none"}, {"web", "../etc"}} {
		if _, _, err := h.m.OpenImage(ctx, tc.app, tc.id); err != ErrNotFound {
			t.Errorf("OpenImage(%q,%q) = %v, want ErrNotFound", tc.app, tc.id, err)
		}
	}
}

func TestFileStore_RejectsUnsafeNames(t *testing.T) {
	fs := NewFileStore(t.TempDir())
	for _, bad := range [][2]string{{"..", "x"}, {"a/b", "x"}, {"a", "../x"}, {"", "x"}, {"a", ""}, {"a", "x/y"}} {
		if _, err := fs.Path(bad[0], bad[1]); err == nil {
			t.Errorf("Path(%q,%q) accepted an unsafe name", bad[0], bad[1])
		}
	}
	if err := fs.Write("app", "dep_ok-1", []byte("x")); err != nil {
		t.Errorf("safe name rejected: %v", err)
	}
}
