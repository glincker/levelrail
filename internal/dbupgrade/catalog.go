package dbupgrade

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

//go:embed catalog.json
var catalogJSON []byte

// Engine automatic upgrade ceilings in catalog.json, besides KindPatch and KindMinor.
const autoNone = "none"

// SupportLine is one release line and its upstream end-of-life date.
type SupportLine struct {
	Line string `json:"line"`
	EOL  string `json:"eol"`
}

// Advisory lists the CVE ids fixed by each version in Fixed; a version is
// affected when it is older than the fixed version of its own line.
type Advisory struct {
	IDs   []string `json:"ids"`
	Fixed []string `json:"fixed"`
}

// EngineCatalog is one engine's curated upgrade data.
type EngineCatalog struct {
	Levels       []string      `json:"levels"`
	AutoMax      string        `json:"auto_max"`
	ImageRevert  bool          `json:"image_revert"`
	ManualReason string        `json:"manual_reason"`
	Notes        string        `json:"notes"`
	Versions     []string      `json:"versions"`
	Lines        []SupportLine `json:"lines"`
	Advisories   []Advisory    `json:"advisories"`
}

type catalogFile struct {
	Updated string                   `json:"updated"`
	Engines map[string]EngineCatalog `json:"engines"`
}

// Catalog is the embedded curated catalog plus any versions a background
// registry refresh discovered since start. Safe for concurrent use.
type Catalog struct {
	mu         sync.RWMutex
	updated    string
	engines    map[string]EngineCatalog
	discovered map[string][]string
}

// LoadCatalog parses the embedded catalog.
func LoadCatalog() (*Catalog, error) {
	return parseCatalog(catalogJSON)
}

func parseCatalog(raw []byte) (*Catalog, error) {
	var f catalogFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("dbupgrade: parse catalog: %w", err)
	}
	for id, e := range f.Engines {
		if len(e.Levels) == 0 {
			return nil, fmt.Errorf("dbupgrade: catalog engine %q has no levels", id)
		}
		for _, v := range e.Versions {
			if _, ok := parseVersion(v); !ok {
				return nil, fmt.Errorf("dbupgrade: catalog engine %q has unparseable version %q", id, v)
			}
		}
	}
	return &Catalog{updated: f.Updated, engines: f.Engines, discovered: map[string][]string{}}, nil
}

// Updated is the date the curated catalog was last edited.
func (c *Catalog) Updated() string { return c.updated }

// Engine returns an engine's catalog with discovered versions merged in.
func (c *Catalog) Engine(id string) (EngineCatalog, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.engines[id]
	if !ok {
		return EngineCatalog{}, false
	}
	extra := c.discovered[id]
	if len(extra) == 0 {
		return e, true
	}
	seen := map[string]bool{}
	merged := make([]string, 0, len(e.Versions)+len(extra))
	for _, v := range append(append([]string{}, e.Versions...), extra...) {
		if !seen[v] {
			seen[v] = true
			merged = append(merged, v)
		}
	}
	sort.Strings(merged)
	e.Versions = merged
	return e, true
}

// SetDiscovered replaces the refreshed versions for one engine.
func (c *Catalog) SetDiscovered(engine string, versions []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.discovered[engine] = append([]string(nil), versions...)
}

// Engines lists every engine id in the catalog.
func (c *Catalog) Engines() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.engines))
	for id := range c.engines {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
