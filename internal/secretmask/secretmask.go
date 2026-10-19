// Package secretmask masks known secret values in text before it is written
// to a log, audit row or stored build output.
package secretmask

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	// Marker replaces a known secret.
	Marker = "[redacted]"
	// MinLength skips short values: masking "true" or "1234" would shred ordinary text.
	MinLength = 8
	maxValues = 20000
)

// Registry holds secret values and rewrites text that contains them.
type Registry struct {
	mu     sync.Mutex
	values map[string]struct{}
	sorted atomic.Pointer[[]string]
}

// New returns an empty registry.
func New() *Registry {
	r := &Registry{values: make(map[string]struct{})}
	empty := []string{}
	r.sorted.Store(&empty)
	return r
}

// Add registers a secret value; short values and a full registry are ignored.
func (r *Registry) Add(v string) {
	if len(v) < MinLength {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.values[v]; ok || len(r.values) >= maxValues {
		return
	}
	r.values[v] = struct{}{}
	list := make([]string, 0, len(r.values))
	for k := range r.values {
		list = append(list, k)
	}
	sort.Slice(list, func(i, j int) bool { return len(list[i]) > len(list[j]) })
	r.sorted.Store(&list)
}

// Mask replaces every registered value in s, longest first.
func (r *Registry) Mask(s string) string {
	list := *r.sorted.Load()
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
func (r *Registry) Len() int { return len(*r.sorted.Load()) }

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
