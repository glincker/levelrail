package catalog

import (
	_ "embed"
	"encoding/json"
)

//go:embed template_meta.json
var templateMetaJSON []byte

// TemplateMeta is marketplace metadata for templates compiled from Go
// literals, which have no template.yaml to carry it.
type TemplateMeta struct {
	SourceRepo   string `json:"source_repo,omitempty"`
	License      string `json:"license,omitempty"`
	LicenseClass string `json:"license_class,omitempty"`
	LicenseURL   string `json:"license_url,omitempty"`
	Description  string `json:"description,omitempty"`
	// Status stays empty for active entries; deprecated ones carry a plain-fact reason.
	Status       string `json:"status,omitempty"`
	StatusReason string `json:"status_reason,omitempty"`
	ReplacedBy   string `json:"replaced_by,omitempty"`
}

var templateMeta = func() map[string]TemplateMeta {
	out := map[string]TemplateMeta{}
	if err := json.Unmarshal(templateMetaJSON, &out); err != nil {
		panic("catalog: template_meta.json: " + err.Error())
	}
	return out
}()

// MetaFor returns the marketplace metadata recorded for a template id.
func MetaFor(id string) (TemplateMeta, bool) {
	m, ok := templateMeta[id]
	return m, ok
}
