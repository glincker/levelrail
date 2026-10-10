package imagemove

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync/atomic"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// ErrTooLarge is returned when the stream passes the size bound.
var ErrTooLarge = errors.New("the image stream passed the size limit")

// Runtime is the target node surface a transfer needs.
type Runtime interface {
	docker.ImageLoader
	docker.ImageInspector
	docker.ImageLayerInspector
}

// Request is one image to move.
type Request struct {
	Ref string
	// WantID is the source image ID; "" means the load cannot be verified.
	WantID   string
	MaxBytes int64
	Progress func(bytes int64)
}

// Result reports what a transfer did.
type Result struct {
	Bytes    int64
	LoadedID string
	// AlreadyPresent is true when the target already had the exact image.
	AlreadyPresent bool
	Verified       bool
	// ConfigDigest is the sha256 of the streamed image config.
	ConfigDigest string
}

// Transfer moves one image and verifies it by content: the loaded image's
// layers must equal the streamed config's diff IDs, and a source image ID
// must match the config digest (classic store) or the loaded ID (same
// store). It skips the stream when the target already holds the same image
// ID, so a re-run resumes instead of re-copying.
func Transfer(ctx context.Context, src Saver, dst Runtime, req Request) (Result, error) {
	if err := ValidateImageRef(req.Ref); err != nil {
		return Result{}, err
	}
	if req.WantID != "" {
		if err := ValidateImageID(req.WantID); err != nil {
			return Result{}, err
		}
		if id, err := dst.InspectImageID(ctx, req.Ref); err == nil && id == req.WantID {
			return Result{LoadedID: id, AlreadyPresent: true, Verified: true}, nil
		}
	}
	rc, err := src.Save(ctx, req.Ref)
	if err != nil {
		return Result{}, err
	}
	cr := &countingReader{r: rc, max: req.MaxBytes, progress: req.Progress}
	scan := startScan(cr, req.Ref)
	loadErr := dst.LoadImage(ctx, scan.r)
	closeErr := rc.Close()
	info, scanErr := scan.finish()
	res := Result{Bytes: cr.n.Load()}
	switch {
	case cr.over:
		return res, fmt.Errorf("%w (%d bytes)", ErrTooLarge, req.MaxBytes)
	case ctx.Err() != nil:
		return res, fmt.Errorf("transfer stopped: %w", ctx.Err())
	case closeErr != nil:
		return res, closeErr
	case loadErr != nil:
		return res, fmt.Errorf("load into the target: %w", loadErr)
	}
	if scanErr != nil {
		return res, fmt.Errorf("verify the image content: %w", scanErr)
	}
	res.ConfigDigest = info.ConfigDigest
	id, err := dst.InspectImageID(ctx, req.Ref)
	if err != nil {
		return res, fmt.Errorf("inspect the loaded image: %w", err)
	}
	if id == "" {
		return res, fmt.Errorf("the target has no image %s after the load", req.Ref)
	}
	res.LoadedID = id
	layers, err := dst.InspectImageLayers(ctx, req.Ref)
	if err != nil {
		return res, fmt.Errorf("inspect the loaded image layers: %w", err)
	}
	if !slices.Equal(layers, info.DiffIDs) {
		return res, fmt.Errorf("the loaded image %s has %d layers that differ from the %d in the source archive", req.Ref, len(layers), len(info.DiffIDs))
	}
	if req.WantID != "" && req.WantID != info.ConfigDigest && req.WantID != id {
		return res, fmt.Errorf("layers match but the source image ID %s equals neither the archive config digest %s nor the loaded image ID %s", req.WantID, info.ConfigDigest, id)
	}
	res.Verified = true
	return res, nil
}

type countingReader struct {
	r        io.Reader
	n        atomic.Int64
	max      int64
	over     bool
	progress func(int64)
}

func (c *countingReader) Read(p []byte) (int, error) {
	if c.max > 0 {
		left := c.max - c.n.Load()
		if left <= 0 {
			c.over = true
			return 0, ErrTooLarge
		}
		if int64(len(p)) > left+1 {
			p = p[:left+1]
		}
	}
	n, err := c.r.Read(p)
	total := c.n.Add(int64(n))
	if c.progress != nil && n > 0 {
		c.progress(total)
	}
	if c.max > 0 && total > c.max {
		c.over = true
		return n, ErrTooLarge
	}
	return n, err
}
