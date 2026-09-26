package preview

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"testing"
)

func TestBuildThumb_ValidCapture(t *testing.T) {
	th, err := BuildThumb(stripedPNG(t), 640, 72, 200<<10, 0.995)
	if err != nil {
		t.Fatalf("BuildThumb: %v", err)
	}
	if th.Width != 640 || th.Height != 400 {
		t.Errorf("size = %dx%d, want 640x400", th.Width, th.Height)
	}
	img, err := jpeg.Decode(bytes.NewReader(th.JPEG))
	if err != nil {
		t.Fatalf("thumbnail is not a JPEG: %v", err)
	}
	if img.Bounds().Dx() != 640 {
		t.Errorf("decoded width = %d", img.Bounds().Dx())
	}
}

func TestBuildThumb_Rejections(t *testing.T) {
	nearBlank := testPNG(t, func(x, y int) color.Color {
		if x < 3 && y < 3 {
			return color.Black
		}
		return color.White
	})
	noisy := testPNG(t, func(int, int) color.Color {
		return color.RGBA{R: uint8(rand.Intn(256)), G: uint8(rand.Intn(256)), B: uint8(rand.Intn(256)), A: 255} //nolint:gosec // test noise, not security
	})
	tests := []struct {
		name     string
		data     []byte
		maxBytes int
		wantBlk  bool
		wantErr  bool
	}{
		{name: "single color", data: flatPNG(t), maxBytes: 200 << 10, wantBlk: true, wantErr: true},
		{name: "near single color", data: nearBlank, maxBytes: 200 << 10, wantBlk: true, wantErr: true},
		{name: "not an image", data: []byte("nope"), maxBytes: 200 << 10, wantErr: true},
		{name: "oversized after re-encode", data: noisy, maxBytes: 1 << 10, wantErr: true},
		{name: "empty", data: nil, maxBytes: 200 << 10, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildThumb(tt.data, 640, 72, tt.maxBytes, 0.995)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if errors.Is(err, ErrBlankImage) != tt.wantBlk {
				t.Errorf("blank = %v, want %v (err %v)", errors.Is(err, ErrBlankImage), tt.wantBlk, err)
			}
		})
	}
}

func TestBuildThumb_RejectsOversizedDimensions(t *testing.T) {
	huge := image.NewRGBA(image.Rect(0, 0, 5000, 10))
	var buf bytes.Buffer
	if err := encodePNG(&buf, huge); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildThumb(buf.Bytes(), 640, 72, 200<<10, 0.995); err == nil {
		t.Error("expected dimension rejection")
	}
}
