package preview

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func paintedImage(w, h int, alpha uint8) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if (x/20+y/20)%2 == 0 {
				img.Set(x, y, color.NRGBA{R: 200, G: 30, B: 30, A: alpha})
			} else {
				img.Set(x, y, color.NRGBA{R: 20, G: 30, B: 200, A: alpha})
			}
		}
	}
	return img
}

func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func jpegBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestBuildThumbFit(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{name: "wide png", data: pngBytes(t, paintedImage(1200, 630, 255))},
		{name: "tall jpeg", data: jpegBytes(t, paintedImage(400, 1000, 255))},
		{name: "small image is not upscaled", data: pngBytes(t, paintedImage(300, 200, 255))},
		{name: "transparent png composites over white", data: pngBytes(t, paintedImage(600, 400, 0)), wantErr: ErrBlankImage},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			th, err := BuildThumbFit(tc.data, 640, 400, 120, 72, 200<<10, 0.995)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			cfg, format, derr := image.DecodeConfig(bytes.NewReader(th.JPEG))
			if derr != nil || format != "jpeg" || cfg.Width != 640 || cfg.Height != 400 || th.Width != 640 || th.Height != 400 {
				t.Errorf("thumb = %dx%d %s err %v, want 640x400 jpeg", cfg.Width, cfg.Height, format, derr)
			}
		})
	}
}

func TestBuildThumbFit_Rejections(t *testing.T) {
	flat := image.NewRGBA(image.Rect(0, 0, 800, 600))
	for i := range flat.Pix {
		flat.Pix[i] = 255
	}
	tests := []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{name: "tiny", data: pngBytes(t, paintedImage(60, 60, 255)), wantErr: ErrTinyImage},
		{name: "thin banner", data: pngBytes(t, paintedImage(1200, 50, 255)), wantErr: ErrTinyImage},
		{name: "single color", data: pngBytes(t, flat), wantErr: ErrBlankImage},
		{name: "not an image", data: []byte("<html>nope</html>")},
		{name: "truncated png", data: pngBytes(t, paintedImage(400, 400, 255))[:80]},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildThumbFit(tc.data, 640, 400, 120, 72, 200<<10, 0.995)
			if err == nil {
				t.Fatal("want an error")
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestBuildThumbFit_OversizedDimensionsAndBytes(t *testing.T) {
	huge := image.NewGray(image.Rect(0, 0, maxSourceDim+1, 200))
	if _, err := BuildThumbFit(pngBytes(t, huge), 640, 400, 120, 72, 200<<10, 0.995); err == nil {
		t.Error("dimensions over the cap must be rejected before decoding")
	}
	if _, err := BuildThumbFit(pngBytes(t, paintedImage(800, 600, 255)), 640, 400, 120, 72, 100, 0.995); err == nil {
		t.Error("a thumbnail over the byte cap must be rejected")
	}
}
