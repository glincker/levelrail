package ingress

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/caddyserver/caddy/v2"

	"github.com/GLINCKER/levelrail/internal/staticroutes"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		for y := range h {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newImageSite(t *testing.T) (*staticSiteModule, []byte) {
	t.Helper()
	dir := t.TempDir()
	src := pngBytes(t, 200, 100)
	files := map[string][]byte{
		"index.html": []byte("HOME"), "pic.png": src, "logo.svg": []byte("<svg/>"), "app.js": []byte("x"),
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rs, err := staticroutes.Compile(staticroutes.Input{}, staticroutes.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	m := &staticSiteModule{Root: dir, Rules: rs}
	if err := m.Provision(caddy.Context{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Cleanup() })
	return m, src
}

func setImageConfig(t *testing.T, cfg ImageResizeConfig) {
	t.Helper()
	SetLiveImageResize(func() ImageResizeConfig { return cfg })
	t.Cleanup(func() { SetLiveImageResize(nil) })
}

func serveImage(t *testing.T, m *staticSiteModule, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	if err := m.ServeHTTP(rec, httptest.NewRequest("GET", target, nil), nil); err != nil {
		t.Fatal(err)
	}
	return rec
}

func decodedWidth(t *testing.T, b []byte) int {
	t.Helper()
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Width
}

var onConfig = ImageResizeConfig{On: true, MaxWidth: 2048, MaxSourceBytes: 1 << 20, MaxPixels: 1 << 20, CacheBytes: 1 << 20}

func TestImageResizeAppliedWhenOn(t *testing.T) {
	m, _ := newImageSite(t)
	setImageConfig(t, onConfig)
	rec := serveImage(t, m, "/pic.png?w=50")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" || decodedWidth(t, rec.Body.Bytes()) != 50 {
		t.Fatalf("code=%d type=%q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Cache-Control") == "" {
		t.Fatal("cache control missing")
	}
	again := serveImage(t, m, "/pic.png?w=50")
	if !bytes.Equal(again.Body.Bytes(), rec.Body.Bytes()) {
		t.Fatal("cached variant differs")
	}
}

func TestImageResizeIgnoredWhenOff(t *testing.T) {
	m, src := newImageSite(t)
	rec := serveImage(t, m, "/pic.png?w=50")
	if !bytes.Equal(rec.Body.Bytes(), src) {
		t.Fatal("original expected when the setting is off")
	}
}

func TestImageResizeOverLimitReturnsOriginal(t *testing.T) {
	m, src := newImageSite(t)
	cfg := onConfig
	cfg.MaxPixels = 100
	setImageConfig(t, cfg)
	if !bytes.Equal(serveImage(t, m, "/pic.png?w=50").Body.Bytes(), src) {
		t.Fatal("pixel cap should serve the original")
	}
	cfg = onConfig
	cfg.MaxSourceBytes = 10
	setImageConfig(t, cfg)
	if !bytes.Equal(serveImage(t, m, "/pic.png?w=50").Body.Bytes(), src) {
		t.Fatal("size cap should serve the original")
	}
	setImageConfig(t, onConfig)
	if !bytes.Equal(serveImage(t, m, "/pic.png?w=500").Body.Bytes(), src) {
		t.Fatal("upscale must serve the original")
	}
}

func TestImageResizeIgnoresNonImages(t *testing.T) {
	m, _ := newImageSite(t)
	setImageConfig(t, onConfig)
	for target, want := range map[string]string{"/logo.svg?w=50": "<svg/>", "/app.js?w=50": "x"} {
		if got := serveImage(t, m, target).Body.String(); got != want {
			t.Fatalf("%s served %q", target, got)
		}
	}
}
