package preview

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"
)

type fakeFetcher struct {
	mu       sync.Mutex
	page     *PageResult
	pageErr  error
	image    []byte
	imageErr error
	pages    int
	images   int
}

func (f *fakeFetcher) FetchPage(context.Context, MetaTarget, string) (*PageResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages++
	return f.page, f.pageErr
}

func (f *fakeFetcher) FetchImage(context.Context, MetaTarget, *url.URL, string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.images++
	return f.image, f.imageErr
}

func pageWith(status int, meta PageMeta) *PageResult {
	base, _ := url.Parse("http://web:3000/")
	return &PageResult{HTTPStatus: status, Base: base, Meta: meta}
}

type metaHarness struct {
	*harness
	fetch *fakeFetcher
}

func newMetaHarness(t *testing.T, mutate func(*Config)) *metaHarness {
	t.Helper()
	f := &fakeFetcher{page: pageWith(200, PageMeta{Title: "Home", Description: "Welcome", ThemeColor: "#102030"})}
	h := newHarness(t, func(c *Config) {
		c.DefaultMode = ModeMetadata
		if mutate != nil {
			mutate(c)
		}
	})
	WithMetaFetcher(f)(h.m)
	return &metaHarness{harness: h, fetch: f}
}

func (h *metaHarness) ready(t *testing.T) {
	t.Helper()
	h.m.NotifyReady("web", "img:1", "id")
	h.waitIdle(t, "web")
}

func TestModeSelection(t *testing.T) {
	str := func(s string) *string { return &s }
	yes, no := true, false
	tests := []struct {
		name        string
		def         Mode
		patch       *SettingsPatch
		wantMode    Mode
		wantEnabled bool
	}{
		{name: "unconfigured app takes the metadata default", def: ModeMetadata, wantMode: ModeMetadata, wantEnabled: true},
		{name: "unconfigured app takes an off default", def: ModeOff, wantMode: ModeOff},
		{name: "unconfigured app takes a screenshot default", def: ModeScreenshot, wantMode: ModeScreenshot, wantEnabled: true},
		{name: "legacy enabled true maps to screenshot", def: ModeMetadata, patch: &SettingsPatch{Enabled: &yes}, wantMode: ModeScreenshot, wantEnabled: true},
		{name: "legacy enabled false maps to off", def: ModeMetadata, patch: &SettingsPatch{Enabled: &no}, wantMode: ModeOff},
		{name: "explicit metadata", def: ModeOff, patch: &SettingsPatch{Mode: str("metadata")}, wantMode: ModeMetadata, wantEnabled: true},
		{name: "explicit off beats the default", def: ModeMetadata, patch: &SettingsPatch{Mode: str("off")}, wantMode: ModeOff},
		{name: "mode wins over enabled", def: ModeOff, patch: &SettingsPatch{Mode: str("metadata"), Enabled: &no}, wantMode: ModeMetadata, wantEnabled: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, func(c *Config) { c.DefaultMode = tc.def })
			if tc.patch != nil {
				if _, err := h.m.SaveSettings(context.Background(), "web", *tc.patch); err != nil {
					t.Fatal(err)
				}
			}
			s, err := h.m.Settings(context.Background(), "web")
			if err != nil || s.Mode != tc.wantMode || s.Enabled != tc.wantEnabled {
				t.Errorf("settings = %+v err %v, want mode %s enabled %v", s, err, tc.wantMode, tc.wantEnabled)
			}
		})
	}
}

func TestModeSelection_RejectsUnknownMode(t *testing.T) {
	h := newHarness(t, nil)
	bad := "full"
	if _, err := h.m.SaveSettings(context.Background(), "web", SettingsPatch{Mode: &bad}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestLegacyEnabledRowMapsToScreenshot(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.DefaultMode = ModeMetadata })
	h.store.settings["web"] = AppSettings{App: "web", Enabled: true, Path: "/"}
	s, _ := h.m.Settings(context.Background(), "web")
	if s.Mode != ModeScreenshot {
		t.Errorf("mode = %q, want screenshot for a row saved before tiers", s.Mode)
	}
}

func TestMetadataTier_OGImageStoredWithoutBrowser(t *testing.T) {
	h := newMetaHarness(t, nil)
	h.fetch.page = pageWith(200, PageMeta{Title: "Home", OGImage: "/og.png"})
	h.fetch.image = pngBytes(t, paintedImage(1200, 630, 255))
	h.ready(t)

	r, ok := h.record("dep_1")
	if !ok || r.Status != StatusOK || r.Source != SourceOGImage || r.Width != 640 || r.Height != 400 || !r.HasFile() {
		t.Fatalf("record = %+v ok %v, want an og_image thumbnail", r, ok)
	}
	if !h.m.fs.Exists("web", "dep_1") {
		t.Error("thumbnail file missing")
	}
	if h.run.started != 0 || h.shoot.calls != 0 || h.store.state[stateImageID] != "" {
		t.Errorf("metadata tier touched the browser: started=%d shots=%d image state %q", h.run.started, h.shoot.calls, h.store.state[stateImageID])
	}
	var meta CardMeta
	if err := json.Unmarshal([]byte(r.Meta), &meta); err != nil || meta.Title != "Home" {
		t.Errorf("meta = %q err %v", r.Meta, err)
	}
}

func TestMetadataTier_CardWhenNoUsableImage(t *testing.T) {
	tests := []struct {
		name  string
		setup func(f *fakeFetcher)
	}{
		{name: "page declares no image", setup: func(*fakeFetcher) {}},
		{name: "image download fails", setup: func(f *fakeFetcher) {
			f.page = pageWith(200, PageMeta{Title: "Home", OGImage: "/og.png"})
			f.imageErr = errBoom
		}},
		{name: "image is tiny", setup: func(f *fakeFetcher) {
			f.page = pageWith(200, PageMeta{Title: "Home", OGImage: "/og.png"})
			f.image = pngBytes(t, paintedImage(40, 40, 255))
		}},
		{name: "image is one color", setup: func(f *fakeFetcher) {
			f.page = pageWith(200, PageMeta{Title: "Home", TwitterImage: "/t.png"})
			f.image = flatPNG(t)
		}},
		{name: "image is not an image", setup: func(f *fakeFetcher) {
			f.page = pageWith(200, PageMeta{Title: "Home", OGImage: "/og.png"})
			f.image = []byte("<html>")
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newMetaHarness(t, nil)
			tc.setup(h.fetch)
			h.ready(t)
			r, ok := h.record("dep_1")
			if !ok || r.Status != StatusOK || r.Source != SourceCard || r.HasFile() || r.Meta == "" {
				t.Fatalf("record = %+v ok %v, want a file-less card", r, ok)
			}
			if h.m.fs.Exists("web", "dep_1") {
				t.Error("a card must not write a file")
			}
		})
	}
}

func TestMetadataTier_SkipsAndFailures(t *testing.T) {
	tests := []struct {
		name       string
		page       *PageResult
		err        error
		wantStatus string
		wantReason string
	}{
		{name: "401 is an auth wall", page: pageWith(401, PageMeta{}), wantStatus: StatusSkipped, wantReason: ReasonAuthWall},
		{name: "403 is an auth wall", page: pageWith(403, PageMeta{}), wantStatus: StatusSkipped, wantReason: ReasonAuthWall},
		{name: "500", page: pageWith(500, PageMeta{}), wantStatus: StatusSkipped, wantReason: ReasonHTTPStatus},
		{name: "offsite redirect", err: errOffsiteRedirect, wantStatus: StatusSkipped, wantReason: ReasonRedirect},
		{name: "not html", err: ErrNotHTML, wantStatus: StatusSkipped, wantReason: ReasonNotHTML},
		{name: "unreachable", err: errBoom, wantStatus: StatusFailed, wantReason: ReasonUnreachable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newMetaHarness(t, nil)
			h.fetch.page, h.fetch.pageErr = tc.page, tc.err
			h.ready(t)
			r, ok := h.record("dep_1")
			if !ok || r.Status != tc.wantStatus || r.Reason != tc.wantReason || r.HasFile() {
				t.Errorf("record = %+v ok %v, want %s/%s", r, ok, tc.wantStatus, tc.wantReason)
			}
			if h.fetch.images != 0 {
				t.Error("no image may be fetched after a refused page")
			}
		})
	}
}

func TestMetadataTier_LoginRedirectIsAnAuthWall(t *testing.T) {
	h := newMetaHarness(t, nil)
	base, _ := url.Parse("http://web:3000/login?next=/")
	h.fetch.page = &PageResult{HTTPStatus: 200, Base: base}
	h.ready(t)
	if r, _ := h.record("dep_1"); r.Reason != ReasonAuthWall {
		t.Errorf("record = %+v, want auth_wall", r)
	}
}

func TestMetadataTier_OffAndScreenshotModes(t *testing.T) {
	t.Run("off does nothing", func(t *testing.T) {
		h := newMetaHarness(t, func(c *Config) { c.DefaultMode = ModeOff })
		h.ready(t)
		if h.fetch.pages != 0 || h.run.started != 0 || len(h.store.records) != 0 {
			t.Errorf("pages=%d started=%d records=%d, want none", h.fetch.pages, h.run.started, len(h.store.records))
		}
	})
	t.Run("screenshot still uses the browser", func(t *testing.T) {
		h := newMetaHarness(t, func(c *Config) { c.DefaultMode = ModeScreenshot })
		h.ready(t)
		r, _ := h.record("dep_1")
		if h.run.started != 1 || h.fetch.pages != 0 || r.Source != SourceScreenshot || !r.HasFile() {
			t.Errorf("started=%d pages=%d record=%+v, want a screenshot and no page fetch", h.run.started, h.fetch.pages, r)
		}
	})
	t.Run("server switch off wins over any mode", func(t *testing.T) {
		h := newMetaHarness(t, func(c *Config) { c.Enabled = false })
		h.ready(t)
		if h.fetch.pages != 0 {
			t.Error("kill switch must stop metadata fetches too")
		}
	})
}

func TestMetadataTier_RecaptureKeepsBetterImage(t *testing.T) {
	h := newMetaHarness(t, nil)
	h.fetch.page = pageWith(200, PageMeta{Title: "Home", OGImage: "/og.png"})
	h.fetch.image = pngBytes(t, paintedImage(1200, 630, 255))
	h.ready(t)
	h.fetch.imageErr = errBoom
	if err := h.m.Capture(context.Background(), "web"); err != nil {
		t.Fatal(err)
	}
	h.waitIdle(t, "web")
	if r, _ := h.record("dep_1"); r.Source != SourceOGImage || !h.m.fs.Exists("web", "dep_1") {
		t.Errorf("record = %+v, a card must not replace an existing site image", r)
	}
}

func TestRetention_AppliesToEverySource(t *testing.T) {
	h := newMetaHarness(t, func(c *Config) { c.KeepPerApp = 2; c.TTL = 0 })
	ctx := context.Background()
	h.enable(t, "web")
	sources := []string{SourceScreenshot, SourceOGImage, SourceCard, SourceCard, SourceOGImage}
	for i, src := range sources {
		id := "old_" + string(rune('a'+i))
		rec := Record{DeploymentID: id, App: "web", Path: "/", Status: StatusOK, Source: src, Bytes: 10, CapturedAt: h.nowVal.Add(time.Duration(i) * time.Minute)}
		if rec.HasFile() {
			if err := h.m.fs.Write("web", id, []byte("jpg")); err != nil {
				t.Fatal(err)
			}
		}
		h.store.records[id] = rec
	}
	h.res.current = "none"
	if _, err := h.m.Prune(ctx, "web", false); err != nil {
		t.Fatal(err)
	}
	if len(h.store.records) != 2 {
		t.Fatalf("records after keep limit = %d, want 2", len(h.store.records))
	}
	for _, id := range []string{"old_a", "old_b", "old_c"} {
		if _, ok := h.record(id); ok || h.m.fs.Exists("web", id) {
			t.Errorf("%s survived retention", id)
		}
	}
	if _, ok := h.record("old_d"); !ok {
		t.Error("newest card record was evicted")
	}
	if !h.m.fs.Exists("web", "old_e") {
		t.Error("newest site image file was removed")
	}
}

func TestRetention_ByteBudgetCountsCardsAndImages(t *testing.T) {
	h := newMetaHarness(t, func(c *Config) { c.KeepPerApp = 10; c.TTL = 0; c.MaxTotalBytes = 25 })
	h.enable(t, "web")
	for i, src := range []string{SourceOGImage, SourceCard, SourceCard} {
		id := "b" + string(rune('0'+i))
		h.store.records[id] = Record{DeploymentID: id, App: "web", Status: StatusOK, Source: src, Bytes: 10, CapturedAt: h.nowVal.Add(time.Duration(i) * time.Minute)}
		if src != SourceCard {
			_ = h.m.fs.Write("web", id, []byte("jpg"))
		}
	}
	h.res.current = "none"
	if _, err := h.m.Prune(context.Background(), "web", false); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.record("b0"); ok {
		t.Error("oldest record must be evicted to fit the budget")
	}
	if len(h.store.records) != 2 {
		t.Errorf("records = %d, want 2", len(h.store.records))
	}
}

func TestSweepAndDelete_HandleCardsAndSiteImages(t *testing.T) {
	h := newMetaHarness(t, nil)
	h.enable(t, "web")
	h.store.records["card"] = Record{DeploymentID: "card", App: "web", Status: StatusOK, Source: SourceCard, Meta: "{}", CapturedAt: h.nowVal}
	h.store.records["og"] = Record{DeploymentID: "og", App: "web", Status: StatusOK, Source: SourceOGImage, CapturedAt: h.nowVal}
	_ = h.m.fs.Write("web", "og", []byte("jpg"))
	h.res.current = "og"

	h.m.Sweep(context.Background())
	if _, ok := h.record("card"); !ok {
		t.Error("sweep must not treat a file-less card as a dangling record")
	}
	if !h.m.fs.Exists("web", "og") {
		t.Error("sweep removed a live site image")
	}

	if err := h.m.DeleteApp(context.Background(), "web"); err != nil {
		t.Fatal(err)
	}
	if len(h.store.records) != 0 || h.m.fs.Exists("web", "og") {
		t.Errorf("app delete left records=%d or files", len(h.store.records))
	}
}
