package staticroutes

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"testing"
)

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestResizeImage(t *testing.T) {
	lim := ImageLimits{MaxWidth: 2048, MaxSourceBytes: 1 << 20, MaxPixels: 1 << 20}
	src := testPNG(t, 200, 100)
	tests := []struct {
		name      string
		src       []byte
		width     int
		lim       ImageLimits
		wantErr   bool
		wantWidth int
	}{
		{"downscale", src, 50, lim, false, 50},
		{"no upscale", src, 400, lim, true, 0},
		{"no width", src, 0, lim, true, 0},
		{"source too big", src, 50, ImageLimits{MaxSourceBytes: 10, MaxPixels: 1 << 20}, true, 0},
		{"too many pixels", src, 50, ImageLimits{MaxSourceBytes: 1 << 20, MaxPixels: 100}, true, 0},
		{"not an image", []byte("hello"), 50, lim, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, ctype, err := ResizeImage(tt.src, tt.width, tt.lim)
			if tt.wantErr {
				if !errors.Is(err, ErrImageUnchanged) {
					t.Fatalf("want unchanged, got %v", err)
				}
				return
			}
			if err != nil || ctype != "image/png" {
				t.Fatalf("err %v type %q", err, ctype)
			}
			cfg, _, err := image.DecodeConfig(bytes.NewReader(out))
			if err != nil || cfg.Width != tt.wantWidth || cfg.Height != 25 {
				t.Fatalf("got %+v err %v", cfg, err)
			}
		})
	}
}

func TestParseResizeWidth(t *testing.T) {
	for raw, want := range map[string]int{"": 0, "abc": 0, "-5": 0, "300": 300, "99999": 2048} {
		if got := ParseResizeWidth(raw, 2048); got != want {
			t.Errorf("%q: got %d want %d", raw, got, want)
		}
	}
}
