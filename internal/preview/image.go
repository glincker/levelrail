package preview

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
)

const (
	maxSourceDim   = 4096
	blankGridSteps = 64
	minQuality     = 30
)

// ErrBlankImage means the capture is a single flat color.
var ErrBlankImage = errors.New("preview: capture is blank")

// Thumb is an encoded thumbnail.
type Thumb struct {
	JPEG          []byte
	Width, Height int
}

// IsBlank reports whether one color (quantized to 4 bits per channel)
// covers at least ratio of a sampled grid of img.
func IsBlank(img image.Image, ratio float64) bool {
	b := img.Bounds()
	if b.Empty() {
		return true
	}
	counts := map[int]int{}
	total := 0
	for gy := 0; gy < blankGridSteps; gy++ {
		y := b.Min.Y + gy*b.Dy()/blankGridSteps
		for gx := 0; gx < blankGridSteps; gx++ {
			x := b.Min.X + gx*b.Dx()/blankGridSteps
			r, g, bl, _ := img.At(x, y).RGBA()
			counts[int(r>>12)<<8|int(g>>12)<<4|int(bl>>12)]++
			total++
		}
	}
	top := 0
	for _, n := range counts {
		top = max(top, n)
	}
	return float64(top)/float64(total) >= ratio
}

// downscale area-averages src to width w, keeping the aspect ratio.
func downscale(src image.Image, w int) *image.RGBA {
	sb := src.Bounds()
	if sb.Dx() <= w {
		w = sb.Dx()
	}
	h := max(1, sb.Dy()*w/sb.Dx())
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		y0 := sb.Min.Y + y*sb.Dy()/h
		y1 := max(y0+1, sb.Min.Y+(y+1)*sb.Dy()/h)
		for x := 0; x < w; x++ {
			x0 := sb.Min.X + x*sb.Dx()/w
			x1 := max(x0+1, sb.Min.X+(x+1)*sb.Dx()/w)
			var r, g, b, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					pr, pg, pb, _ := src.At(sx, sy).RGBA()
					r += uint64(pr)
					g += uint64(pg)
					b += uint64(pb)
					n++
				}
			}
			dst.SetRGBA(x, y, color.RGBA{R: uint8(r / n >> 8), G: uint8(g / n >> 8), B: uint8(b / n >> 8), A: 255}) //nolint:gosec // averaged 16-bit channel shifted to 8 bits fits uint8
		}
	}
	return dst
}

// BuildThumb validates a PNG capture and returns a JPEG thumbnail of width
// thumbWidth. It re-encodes once at lower quality when the result exceeds
// maxBytes, and rejects it if it is still too large.
func BuildThumb(pngData []byte, thumbWidth, quality, maxBytes int, blankRatio float64) (*Thumb, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(pngData))
	if err != nil {
		return nil, fmt.Errorf("preview: decode capture header: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxSourceDim || cfg.Height > maxSourceDim {
		return nil, fmt.Errorf("preview: capture dimensions %dx%d out of bounds", cfg.Width, cfg.Height)
	}
	src, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, fmt.Errorf("preview: decode capture: %w", err)
	}
	if IsBlank(src, blankRatio) {
		return nil, ErrBlankImage
	}
	thumb := downscale(src, thumbWidth)
	for _, q := range []int{quality, max(minQuality, quality-25)} {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: q}); err != nil {
			return nil, fmt.Errorf("preview: encode thumbnail: %w", err)
		}
		if maxBytes <= 0 || buf.Len() <= maxBytes {
			return &Thumb{JPEG: buf.Bytes(), Width: thumb.Bounds().Dx(), Height: thumb.Bounds().Dy()}, nil
		}
	}
	return nil, fmt.Errorf("preview: thumbnail exceeds %d bytes", maxBytes)
}
