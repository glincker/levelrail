package demo

import (
	"bytes"
	"image/jpeg"
	"testing"
)

func TestGradientJPEG_IsADecodableScreenshot(t *testing.T) {
	data, err := gradientJPEG(demoShots[0].from, demoShots[0].to)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Dx() != demoShotW || img.Bounds().Dy() != demoShotH {
		t.Fatalf("decode = %v %v, want %dx%d", img, err, demoShotW, demoShotH)
	}
}
