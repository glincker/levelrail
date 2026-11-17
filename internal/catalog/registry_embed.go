package catalog

import (
	"io/fs"

	"github.com/GLINCKER/levelrail/registry"
)

// registryOnlyTemplates are registry ids no Go source, contrib file or manifest
// defines. The compiled catalog wins on overlap, so existing entries never change.
var registryOnlyTemplates, registryOnlyMeta = mustLoadRegistryOnly()

// heldBack lists registry-only ids that fail a catalog guard today. They stay
// unserved until their registry compose is fixed, then the entry is deleted here.
var heldBack = map[string]string{}

func mustLoadRegistryOnly() ([]Template, map[string]TemplateMeta) {
	sub, err := fs.Sub(registry.Templates, "templates")
	if err != nil {
		panic(err)
	}
	entries, err := LoadRegistryEntriesFS(sub)
	if err != nil {
		panic(err)
	}
	have := map[string]bool{}
	for _, t := range compiledTemplates {
		have[t.ID] = true
	}
	var out []Template
	meta := map[string]TemplateMeta{}
	for _, e := range entries {
		if _, held := heldBack[e.Template.ID]; held || have[e.Template.ID] {
			continue
		}
		out = append(out, e.Template)
		meta[e.Template.ID] = TemplateMeta{
			SourceRepo: e.Meta.SourceRepo, License: e.Meta.License, LicenseClass: e.Meta.LicenseClass,
			LicenseURL: e.Meta.LicenseURL, Status: e.Meta.Status, Description: e.Meta.Description,
		}
	}
	return out, meta
}
