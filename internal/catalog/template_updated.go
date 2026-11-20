package catalog

import (
	_ "embed"
	"encoding/json"
)

// template_updated.json maps a template id to the date its files last changed.
// It is written by scripts/gen-template-updated from git history, never by hand.
//
//go:embed template_updated.json
var templateUpdatedJSON []byte

var templateUpdated = func() map[string]string {
	out := map[string]string{}
	if err := json.Unmarshal(templateUpdatedJSON, &out); err != nil {
		panic("catalog: template_updated.json: " + err.Error())
	}
	return out
}()

// UpdatedAtFor returns the recorded last-updated date of a template, or an
// empty string when none is recorded.
func UpdatedAtFor(id string) string {
	return templateUpdated[id]
}
