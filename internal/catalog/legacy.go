package catalog

import (
	_ "embed"
	"strings"
)

//go:embed legacy_manifests.txt
var legacyManifestsTxt string

// LegacyManifestIDs are manifests that predate the marketplace rules. The
// list only shrinks: bringing one up to the rules means deleting its line.
func LegacyManifestIDs() map[string]bool {
	out := map[string]bool{}
	for _, id := range strings.Fields(legacyManifestsTxt) {
		out[id] = true
	}
	return out
}

// TombstoneEntry is a removed id that must never be reused.
type TombstoneEntry struct {
	ID         string `json:"id"`
	ReplacedBy string `json:"replaced_by,omitempty"`
	Reason     string `json:"reason"`
	RemovedIn  string `json:"removed_in"`
}
