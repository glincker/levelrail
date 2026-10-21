package ingress

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/staticroutes"
)

func init() {
	caddy.RegisterModule(staticSiteModule{})
}

// StaticSiteHandler is the wire shape of levelrail_static_site
// (http.handlers.levelrail_static_site): it serves a published tree and
// applies that deploy's compiled routing rules, so rules change only when
// the tree does.
type StaticSiteHandler struct {
	Handler string `json:"handler"`
	staticSiteModule
}

// NewStaticSiteHandler builds the handler for root and its rules.
func NewStaticSiteHandler(root string, rules *staticroutes.Ruleset) StaticSiteHandler {
	return StaticSiteHandler{Handler: "levelrail_static_site", staticSiteModule: staticSiteModule{Root: root, Rules: rules}}
}

type staticSiteModule struct {
	Root  string                `json:"root"`
	Rules *staticroutes.Ruleset `json:"rules"`

	root *os.Root
}

func (staticSiteModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.levelrail_static_site",
		New: func() caddy.Module { return new(staticSiteModule) },
	}
}

// Provision opens the tree confined, so a symlink inside it cannot reach
// another path on the host.
func (m *staticSiteModule) Provision(caddy.Context) error {
	if m.Rules == nil {
		return fmt.Errorf("static site %q has no rules", m.Root)
	}
	if err := m.Rules.Prepare(); err != nil {
		return err
	}
	root, err := os.OpenRoot(m.Root)
	if err != nil {
		return fmt.Errorf("open static root %q: %w", m.Root, err)
	}
	m.root = root
	return nil
}

func (m *staticSiteModule) Cleanup() error {
	if m.root == nil {
		return nil
	}
	return m.root.Close()
}

func (m *staticSiteModule) ServeHTTP(w http.ResponseWriter, r *http.Request, _ caddyhttp.Handler) error {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return nil
	}
	d := m.Rules.Resolve(m.root, staticroutes.Request{Method: r.Method, Path: r.URL.Path, RawQuery: r.URL.RawQuery})
	h := w.Header()
	for _, kv := range d.Headers {
		h.Set(kv.Name, kv.Value)
	}
	for _, name := range d.Remove {
		h.Del(name)
	}
	if d.Kind == staticroutes.KindRedirect {
		h.Set("Location", d.Location)
		w.WriteHeader(d.Status)
		return nil
	}
	if d.File == "" {
		http.Error(w, http.StatusText(d.Status), d.Status)
		return nil
	}
	f, err := m.root.Open(d.File)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return nil
	}
	defer f.Close() //nolint:errcheck // read-only handle
	fi, err := f.Stat()
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return nil
	}
	if d.Status == http.StatusOK {
		if h.Get("Etag") == "" {
			h.Set("Etag", fmt.Sprintf(`"%x-%x"`, fi.ModTime().UnixNano(), fi.Size()))
		}
		http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
		return nil
	}
	if ct := mime.TypeByExtension(path.Ext(d.File)); ct != "" {
		h.Set("Content-Type", ct)
	}
	h.Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
	w.WriteHeader(d.Status)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, f)
	}
	return nil
}

var (
	_ caddyhttp.MiddlewareHandler = (*staticSiteModule)(nil)
	_ caddy.Provisioner           = (*staticSiteModule)(nil)
	_ caddy.CleanerUpper          = (*staticSiteModule)(nil)
)
