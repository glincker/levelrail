package ingress

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/GLINCKER/levelrail/internal/staticroutes"
)

// ImageResizeConfig is the live resize policy, read per request.
type ImageResizeConfig struct {
	On             bool
	MaxWidth       int
	MaxSourceBytes int
	MaxPixels      int
	CacheBytes     int
}

var liveImageResize atomic.Pointer[func() ImageResizeConfig]

// SetLiveImageResize installs the saved-setting source so this package does
// not import the settings layer. Nil turns resizing off.
func SetLiveImageResize(f func() ImageResizeConfig) {
	if f == nil {
		liveImageResize.Store(nil)
		return
	}
	liveImageResize.Store(&f)
}

func imageResizeConfig() ImageResizeConfig {
	if f := liveImageResize.Load(); f != nil {
		return (*f)()
	}
	return ImageResizeConfig{}
}

type resizedVariant struct {
	body        []byte
	contentType string
}

// variantCache holds resized bytes up to a byte budget, dropping the oldest first.
type variantCache struct {
	mu    sync.Mutex
	items map[string]resizedVariant
	order []string
	used  int
}

var resizedVariants = &variantCache{items: map[string]resizedVariant{}}

func (c *variantCache) get(key string) (resizedVariant, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.items[key]
	return v, ok
}

func (c *variantCache) put(key string, v resizedVariant, budget int) {
	if len(v.body) > budget {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[key]; ok {
		return
	}
	c.items[key] = v
	c.order = append(c.order, key)
	c.used += len(v.body)
	for c.used > budget && len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		c.used -= len(c.items[oldest].body)
		delete(c.items, oldest)
	}
}

// serveResized answers a ?w= request for a JPEG or PNG when resizing is on.
// It returns false to let the caller serve the original file.
func (m *staticSiteModule) serveResized(w http.ResponseWriter, r *http.Request, f *os.File, fi os.FileInfo, file string) bool {
	raw := r.URL.Query().Get("w")
	if raw == "" {
		return false
	}
	cfg := imageResizeConfig()
	if !cfg.On {
		return false
	}
	switch strings.ToLower(path.Ext(file)) {
	case ".jpg", ".jpeg", ".png":
	default:
		return false
	}
	width := staticroutes.ParseResizeWidth(raw, cfg.MaxWidth)
	if width == 0 || fi.Size() > int64(cfg.MaxSourceBytes) {
		return false
	}
	key := fmt.Sprintf("%s|%s|%d|%d|%d", m.Root, file, fi.ModTime().UnixNano(), fi.Size(), width)
	v, ok := resizedVariants.get(key)
	if !ok {
		src, err := io.ReadAll(io.LimitReader(f, int64(cfg.MaxSourceBytes)+1))
		if err != nil {
			return false
		}
		out, ct, err := staticroutes.ResizeImage(src, width, staticroutes.ImageLimits{
			MaxWidth: cfg.MaxWidth, MaxSourceBytes: cfg.MaxSourceBytes, MaxPixels: cfg.MaxPixels,
		})
		if err != nil {
			// Unchanged or failed: the caller serves the original (ServeContent seeks itself).
			return false
		}
		v = resizedVariant{body: out, contentType: ct}
		resizedVariants.put(key, v, cfg.CacheBytes)
	}
	h := w.Header()
	h.Set("Content-Type", v.contentType)
	h.Set("Etag", fmt.Sprintf(`"%x-%x-w%d"`, fi.ModTime().UnixNano(), fi.Size(), width))
	http.ServeContent(w, r, fi.Name(), fi.ModTime(), bytes.NewReader(v.body))
	return true
}
