// Package pages holds the named performance presets for static sites.
package pages

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/glincker/levelrail/internal/spec"
)

//go:embed presets.json
var presetsJSON []byte

// SourcePrefix labels preset header blocks so they can be replaced and ranked.
const SourcePrefix = "preset:"

// HeaderRule is one path pattern with the headers the preset sets on it.
type HeaderRule struct {
	Path string            `json:"path"`
	Set  map[string]string `json:"set"`
}

// Preset is one named bundle of cache, compression and security header choices.
type Preset struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Compression []string          `json:"compression"`
	Headers     []HeaderRule      `json:"headers"`
	Security    map[string]string `json:"security"`
}

var (
	loadOnce sync.Once
	loaded   []Preset
	loadErr  error
)

// All returns the embedded presets in file order.
func All() ([]Preset, error) {
	loadOnce.Do(func() {
		var f struct {
			Presets []Preset `json:"presets"`
		}
		if err := json.Unmarshal(presetsJSON, &f); err != nil {
			loadErr = fmt.Errorf("pages: decode presets: %w", err)
			return
		}
		loaded = f.Presets
	})
	return loaded, loadErr
}

// Get returns the preset with the given id.
func Get(id string) (Preset, bool) {
	all, err := All()
	if err != nil {
		return Preset{}, false
	}
	for _, p := range all {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// userSetNames lists the header names any non-preset block sets, lower-cased.
func userSetNames(blocks []spec.StaticHeaderBlock) map[string]bool {
	out := map[string]bool{}
	for _, b := range blocks {
		if strings.HasPrefix(b.Source, SourcePrefix) {
			continue
		}
		for _, kv := range b.Set {
			out[strings.ToLower(kv.Name)] = true
		}
	}
	return out
}

// Apply replaces any earlier preset blocks in rs with the preset's, ranked after
// every other block. A header name the user sets anywhere is left to the user.
// An empty id only removes the preset.
func Apply(rs *spec.StaticRuleset, id string, l spec.StaticLimits) error {
	kept := make([]spec.StaticHeaderBlock, 0, len(rs.Headers))
	for _, b := range rs.Headers {
		if !strings.HasPrefix(b.Source, SourcePrefix) {
			kept = append(kept, b)
		}
	}
	rs.Headers = kept
	if id == "" {
		return nil
	}
	p, ok := Get(id)
	if !ok {
		return fmt.Errorf("unknown preset %q", id)
	}
	mine := userSetNames(kept)
	for i, h := range p.Headers {
		set := map[string]string{}
		for n, v := range h.Set {
			set[n] = v
		}
		if h.Path == "/*" {
			for n, v := range p.Security {
				set[n] = v
			}
		}
		b := spec.StaticHeaderBlock{Source: fmt.Sprintf("%s%s[%d]", SourcePrefix, p.ID, i), Path: h.Path}
		names := make([]string, 0, len(set))
		for n := range set {
			if !mine[strings.ToLower(n)] {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		for _, n := range names {
			b.Set = append(b.Set, spec.StaticHeaderKV{Name: n, Value: set[n]})
		}
		if len(b.Set) == 0 {
			continue
		}
		if err := spec.ValidateStaticHeaderBlock(&b, l); err != nil {
			return fmt.Errorf("preset %s: %w", p.ID, err)
		}
		rs.Headers = append(rs.Headers, b)
	}
	return nil
}
