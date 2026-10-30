package api

import (
	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/experimental"
)

// templateGates maps a catalog template ID to the experimental flag it needs.
// The static catalog keeps the entry (docs and generators read it); only the
// API listing, lookup and install hide it while the flag is off.
var templateGates = map[string]experimental.Feature{
	"mail-mox": experimental.Mail,
}

// templateAvailable reports whether a catalog template may be listed or installed now.
func templateAvailable(id string) bool {
	f, gated := templateGates[id]
	return !gated || experimental.Enabled(f)
}

// availableTemplates is catalog.Templates minus the gated entries that are off.
func availableTemplates() []catalog.Template {
	out := make([]catalog.Template, 0, len(catalog.Templates))
	for _, tpl := range catalog.Templates {
		if templateAvailable(tpl.ID) {
			out = append(out, tpl)
		}
	}
	return out
}
