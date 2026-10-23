package email

import (
	"fmt"
	"sort"
)

// Builder turns a Config into the Sender for one provider.
type Builder func(cfg Config) (Sender, error)

var registry = map[Backend]Builder{}

// Register adds a provider to the registry; each provider file calls it from init, so adding one is one file.
func Register(b Backend, fn Builder) {
	if _, dup := registry[b]; dup {
		panic(fmt.Sprintf("email: provider %q registered twice", b))
	}
	registry[b] = fn
}

// Backends lists every registered provider in a stable order.
func Backends() []Backend {
	out := make([]Backend, 0, len(registry))
	for b := range registry {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ValidBackend reports whether b is a registered provider.
func ValidBackend(b Backend) bool {
	_, ok := registry[b]
	return ok
}
