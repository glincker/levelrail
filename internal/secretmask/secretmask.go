// Package secretmask masks known secret values in text before it is written
// to a log, audit row or stored build output.
package secretmask

import (
	"context"
	"encoding/base64"
	"log/slog"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	// Marker replaces a known secret.
	Marker = "[redacted]"
	// MinLength skips short values: masking "true" or "1234" would shred ordinary text.
	MinLength = 8
	// defaultMaxPrimaries caps distinct secrets; override with APP_SECRETMASK_MAX_VALUES.
	defaultMaxPrimaries = 20000
	maxValuesEnv        = "APP_SECRETMASK_MAX_VALUES"
)

func maxPrimaries() int {
	if n, err := strconv.Atoi(os.Getenv(maxValuesEnv)); err == nil && n > 0 {
		return n
	}
	return defaultMaxPrimaries
}

// Registry holds secret values and rewrites text that contains them.
type Registry struct {
	mu        sync.Mutex
	primaries map[string]struct{}
	forms     map[string]struct{}
	max       int
	warned    atomic.Bool
	dirty     atomic.Bool
	sorted    atomic.Pointer[[]string]
}

// New returns an empty registry.
func New() *Registry {
	r := &Registry{primaries: make(map[string]struct{}), forms: make(map[string]struct{}), max: maxPrimaries()}
	empty := []string{}
	r.sorted.Store(&empty)
	return r
}

// Add registers a secret value; short values are ignored. The cap counts primary
// values only, encoded forms ride along, and a primary is never evicted.
func (r *Registry) Add(v string) {
	if len(v) < MinLength {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.primaries[v]; ok {
		return
	}
	if len(r.primaries) >= r.max {
		if r.warned.CompareAndSwap(false, true) {
			slog.Warn("secretmask: registry full, new secrets are not masked", slog.Int("primaries", len(r.primaries)))
		}
		return
	}
	// Tools echo a secret base64 or URL encoded (auth headers, DSNs), so those forms are masked too.
	for _, form := range encodedForms(v) {
		r.forms[form] = struct{}{}
	}
	r.primaries[v] = struct{}{}
	// Sorting is deferred to the next Mask so a burst of Adds (startup preload) pays once.
	r.dirty.Store(true)
}

func (r *Registry) snapshot() []string {
	if r.dirty.Load() {
		r.mu.Lock()
		if r.dirty.Load() {
			list := make([]string, 0, len(r.primaries)+len(r.forms))
			for k := range r.primaries {
				list = append(list, k)
			}
			for k := range r.forms {
				if _, isPrimary := r.primaries[k]; !isPrimary {
					list = append(list, k)
				}
			}
			sort.Slice(list, func(i, j int) bool { return len(list[i]) > len(list[j]) })
			r.sorted.Store(&list)
			// Cleared only after the store so a racing Mask never sees clean with a stale list.
			r.dirty.Store(false)
		}
		r.mu.Unlock()
	}
	return *r.sorted.Load()
}

func encodedForms(v string) []string {
	forms := []string{
		base64.StdEncoding.EncodeToString([]byte(v)),
		base64.RawStdEncoding.EncodeToString([]byte(v)),
		base64.URLEncoding.EncodeToString([]byte(v)),
		base64.RawURLEncoding.EncodeToString([]byte(v)),
		url.QueryEscape(v),
		url.PathEscape(v),
	}
	out := forms[:0]
	for _, f := range forms {
		if f != v && len(f) >= MinLength {
			out = append(out, f)
		}
	}
	return out
}

// Mask replaces every registered value in s, longest first.
func (r *Registry) Mask(s string) string {
	list := r.snapshot()
	if len(list) == 0 || s == "" {
		return s
	}
	for _, v := range list {
		if strings.Contains(s, v) {
			s = strings.ReplaceAll(s, v, Marker)
		}
	}
	return s
}

// Len reports how many values are registered.
func (r *Registry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.primaries)
}

var shared = New()

// Add registers v in the process-wide registry.
func Add(v string) { shared.Add(v) }

// Mask masks s with the process-wide registry.
func Mask(s string) string { return shared.Mask(s) }

// Handler wraps next so every string attribute and message is masked.
func Handler(next slog.Handler) slog.Handler { return &handler{next: next} }

type handler struct{ next slog.Handler }

func (h *handler) Enabled(ctx context.Context, l slog.Level) bool { return h.next.Enabled(ctx, l) }

func (h *handler) Handle(ctx context.Context, rec slog.Record) error {
	out := slog.NewRecord(rec.Time, rec.Level, Mask(rec.Message), rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(maskAttr(a))
		return true
	})
	return h.next.Handle(ctx, out)
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	masked := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		masked[i] = maskAttr(a)
	}
	return &handler{next: h.next.WithAttrs(masked)}
}

func (h *handler) WithGroup(name string) slog.Handler { return &handler{next: h.next.WithGroup(name)} }

func maskAttr(a slog.Attr) slog.Attr {
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return slog.String(a.Key, Mask(v.String()))
	case slog.KindGroup:
		group := v.Group()
		out := make([]any, len(group))
		for i, g := range group {
			out[i] = maskAttr(g)
		}
		return slog.Group(a.Key, out...)
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return slog.String(a.Key, Mask(err.Error()))
		}
	}
	return slog.Attr{Key: a.Key, Value: v}
}
