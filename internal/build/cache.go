package build

// This file: TASKS.md 3.5's remote-cache backend. WithCacheDir's own doc
// comment (client.go) already named this as the missing piece, not
// invented fresh here: "a cache genuinely shared across dedicated build
// nodes needs a registry or object-store backend... neither of which
// exist before Phase 3." CacheConfig is that backend, added alongside
// the local one rather than replacing it, because a single build node
// still benefits from a local cache (no registry round trip) even once
// a registry backend is configured for the multi-node case.

import (
	"errors"
	"fmt"

	bkclient "github.com/moby/buildkit/client"
)

// CacheConfig describes every BuildKit cache import/export backend a
// Client should use. The zero value disables caching entirely, matching
// WithCacheDir's pre-existing "empty disables cache import/export
// entirely" behavior.
type CacheConfig struct {
	// Dir enables BuildKit's "local" cache backend: a directory on this
	// process's own disk. Only ever useful to the one build node that
	// owns that disk, which is exactly Phase 1's scoped-down cache
	// (client.go's WithCacheDir).
	Dir string

	// RegistryRef enables BuildKit's "registry" cache backend: an image
	// reference (e.g. "registry.example.com/levelrail/build-cache:app")
	// that cache blobs are pushed to and pulled from using the same
	// registry credentials Docker itself already has configured. This
	// is the actual "remote cache... shared across dedicated build
	// nodes" CLAUDE.md 4.4 and TASKS.md 3.5 both call for: any build
	// node with network access to the registry gets the same cache,
	// unlike Dir.
	RegistryRef string

	// RegistryInsecure allows the registry backend to talk plain HTTP
	// or accept a self-signed certificate, matching BuildKit's own
	// "insecure" cache attribute. Only meaningful when RegistryRef is
	// set; ignored otherwise.
	RegistryInsecure bool
}

// ErrCacheRegistryRefRequired is returned by CacheConfig.entries when
// RegistryInsecure is set but RegistryRef is empty: an insecure flag
// with nothing to apply it to is almost certainly a caller mistake
// (e.g. a typo'd env var name that leaves RegistryRef unset), not a
// meaningful configuration, so this fails loudly at construction time
// rather than silently doing nothing with the flag.
var ErrCacheRegistryRefRequired = errors.New("build: cache registry insecure flag set without a registry ref")

// empty reports whether c disables caching entirely.
func (c CacheConfig) empty() bool {
	return c.Dir == "" && c.RegistryRef == ""
}

// validate checks c is internally consistent before any entries are
// built from it. It does not check reachability: an unreachable
// registry is a build-time failure (BuildKit's Solve call), not a
// configuration error newSolveOpt can catch up front, the same
// "Validate doesn't check the filesystem" split Request.Validate
// documents for the same reason.
func (c CacheConfig) validate() error {
	if c.RegistryInsecure && c.RegistryRef == "" {
		return ErrCacheRegistryRefRequired
	}
	return nil
}

// entries builds the BuildKit CacheOptionsEntry values for both import
// and export, one pair per configured backend. Import and export always
// mirror each other here (every backend that's read from is also
// written to), the same choice the pre-existing local-only cache
// already made: a build node with a cache backend configured always
// tries to both use and improve it.
func (c CacheConfig) entries() (imports, exports []bkclient.CacheOptionsEntry, err error) {
	if err := c.validate(); err != nil {
		return nil, nil, err
	}

	if c.Dir != "" {
		imports = append(imports, bkclient.CacheOptionsEntry{
			Type:  "local",
			Attrs: map[string]string{"src": c.Dir},
		})
		exports = append(exports, bkclient.CacheOptionsEntry{
			Type:  "local",
			Attrs: map[string]string{"dest": c.Dir, "mode": "max"},
		})
	}

	if c.RegistryRef != "" {
		attrs := map[string]string{"ref": c.RegistryRef, "mode": "max"}
		if c.RegistryInsecure {
			attrs["insecure"] = "true"
		}
		imports = append(imports, bkclient.CacheOptionsEntry{Type: "registry", Attrs: attrs})
		exports = append(exports, bkclient.CacheOptionsEntry{Type: "registry", Attrs: attrs})
	}

	return imports, exports, nil
}

// String renders c for logging, never including credentials (this
// struct never holds any: registry auth comes from Docker's own
// configured credential store, the same as every other registry
// operation in this codebase, per ensureImage in internal/docker).
func (c CacheConfig) String() string {
	switch {
	case c.Dir != "" && c.RegistryRef != "":
		return fmt.Sprintf("dir=%s registry=%s", c.Dir, c.RegistryRef)
	case c.Dir != "":
		return fmt.Sprintf("dir=%s", c.Dir)
	case c.RegistryRef != "":
		return fmt.Sprintf("registry=%s", c.RegistryRef)
	default:
		return "disabled"
	}
}
