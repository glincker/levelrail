package demo

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"path/filepath"
	"time"

	"github.com/glincker/levelrail/internal/preview"
)

const (
	demoShotW = 640
	demoShotH = 360
)

// demoShots are the web apps that get a generated placeholder screenshot.
var demoShots = []struct {
	app      string
	from, to color.RGBA
}{
	{"storefront-web", color.RGBA{R: 37, G: 99, B: 235, A: 255}, color.RGBA{R: 236, G: 72, B: 153, A: 255}},
	{"marketing-site", color.RGBA{R: 16, G: 185, B: 129, A: 255}, color.RGBA{R: 14, G: 116, B: 144, A: 255}},
	{"docs-site", color.RGBA{R: 99, G: 102, B: 241, A: 255}, color.RGBA{R: 30, G: 41, B: 59, A: 255}},
}

// gradientJPEG draws a small diagonal gradient with a light header band, so a
// demo card reads like a page without committing any binary file.
func gradientJPEG(from, to color.RGBA) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, demoShotW, demoShotH))
	for y := 0; y < demoShotH; y++ {
		for x := 0; x < demoShotW; x++ {
			t := float64(x+y) / float64(demoShotW+demoShotH)
			c := color.RGBA{
				R: uint8(float64(from.R)*(1-t) + float64(to.R)*t),
				G: uint8(float64(from.G)*(1-t) + float64(to.G)*t),
				B: uint8(float64(from.B)*(1-t) + float64(to.B)*t),
				A: 255,
			}
			if y < 36 {
				c = color.RGBA{R: 248, G: 250, B: 252, A: 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 70}); err != nil {
		return nil, fmt.Errorf("demo: encode placeholder screenshot: %w", err)
	}
	return buf.Bytes(), nil
}

// previewShots seeds ready screenshots for three web apps and a remembered
// failure for one more, so the cards show both states. It needs a DataDir for
// the image files.
func (s *seeder) previewShots() error {
	if s.opts.DataDir == "" {
		return nil
	}
	st := preview.NewSQLStore(s.db)
	files := preview.NewFileStore(filepath.Join(s.opts.DataDir, "previews"))
	at := s.at(0, 2*time.Hour)
	for _, shot := range demoShots {
		data, err := gradientJPEG(shot.from, shot.to)
		if err != nil {
			return err
		}
		id := "dep_demo_shot_" + shot.app
		if err := files.Write(shot.app, id, data); err != nil {
			return fmt.Errorf("demo: write placeholder screenshot: %w", err)
		}
		rec := preview.Record{
			DeploymentID: id, App: shot.app, Path: "/", Bytes: int64(len(data)), Width: demoShotW, Height: demoShotH,
			Status: preview.StatusOK, Source: preview.SourceScreenshot, HTTPStatus: 200, CapturedAt: at,
		}
		if err := st.UpsertPreviewRecord(s.ctx, rec); err != nil {
			return fmt.Errorf("demo: save preview record: %w", err)
		}
	}
	if err := st.SetPreviewState(s.ctx, "render_fail:admin-panel", preview.ReasonAuthWall); err != nil {
		return fmt.Errorf("demo: save preview failure: %w", err)
	}
	return nil
}
