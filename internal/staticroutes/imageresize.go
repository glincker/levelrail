package staticroutes

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"strconv"
)

// ImageLimits bounds one resize. The caller fills it from runtime settings.
type ImageLimits struct {
	MaxWidth       int
	MaxSourceBytes int
	MaxPixels      int
}

// ErrImageUnchanged means the original bytes should be served as they are.
var ErrImageUnchanged = errors.New("staticroutes: image left unchanged")

// ParseResizeWidth reads the w query value. It returns 0 when absent or not a
// positive whole number, and clamps to maxWidth.
func ParseResizeWidth(raw string, maxWidth int) int {
	w, err := strconv.Atoi(raw)
	if err != nil || w <= 0 {
		return 0
	}
	return min(w, maxWidth)
}

// ResizeImage scales a JPEG or PNG to width keeping the aspect ratio. It
// checks size and pixel bounds from the header before decoding, and returns
// ErrImageUnchanged when the file is too big, not an image, or already small.
func ResizeImage(src []byte, width int, lim ImageLimits) ([]byte, string, error) {
	if width <= 0 || len(src) > lim.MaxSourceBytes {
		return nil, "", ErrImageUnchanged
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil || (format != "jpeg" && format != "png") {
		return nil, "", ErrImageUnchanged
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > lim.MaxPixels || width >= cfg.Width {
		return nil, "", ErrImageUnchanged
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, "", ErrImageUnchanged
	}
	height := max(1, cfg.Height*width/cfg.Width)
	out := scaleBox(img, width, height)
	var buf bytes.Buffer
	if format == "png" {
		if err := png.Encode(&buf, out); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), "image/png", nil
	}
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 85}); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "image/jpeg", nil
}

// chan8 narrows a 16-bit color channel to 8 bits.
func chan8(v uint64) uint8 { return uint8(min(v>>8, 255)) }

// scaleBox averages the source pixels under each target pixel, which keeps
// downscaled photos smooth without an imaging dependency.
func scaleBox(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sw, sh := b.Dx(), b.Dy()
	for y := range h {
		y0, y1 := y*sh/h, max((y+1)*sh/h, y*sh/h+1)
		for x := range w {
			x0, x1 := x*sw/w, max((x+1)*sw/w, x*sw/w+1)
			var r, g, bl, a, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					pr, pg, pb, pa := src.At(b.Min.X+sx, b.Min.Y+sy).RGBA()
					r, g, bl, a, n = r+uint64(pr), g+uint64(pg), bl+uint64(pb), a+uint64(pa), n+1
				}
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = chan8(r/n), chan8(g/n), chan8(bl/n), chan8(a/n)
		}
	}
	return dst
}
