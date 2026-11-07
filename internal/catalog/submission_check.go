package catalog

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// CheckSubmission validates a community submission given as the raw
// template.yaml and compose.yaml bodies, with the same loader and marketplace
// rules the catalog applies to a merged template. It never touches the network.
func CheckSubmission(templateYAML, composeYAML []byte) (Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(templateYAML, &m); err != nil {
		return Manifest{}, fmt.Errorf("template.yaml: %w", err)
	}
	if len(composeYAML) == 0 {
		return m, errors.New("compose.yaml is empty")
	}
	m.Compose = string(composeYAML)
	var errs []error
	if err := m.Validate(); err != nil {
		errs = append(errs, err)
	}
	if err := m.ValidateSubmission(); err != nil {
		errs = append(errs, err)
	}
	return m, errors.Join(errs...)
}

// SearchTemplates returns the templates whose id, name, category or slogan
// contain every word of query, case-insensitively, in catalog order.
func SearchTemplates(all []Template, query string) []Template {
	words := strings.Fields(strings.ToLower(query))
	var out []Template
	for _, t := range all {
		hay := strings.ToLower(t.ID + " " + t.Name + " " + t.Category + " " + t.Slogan)
		match := len(words) > 0
		for _, w := range words {
			if !strings.Contains(hay, w) {
				match = false
				break
			}
		}
		if match {
			out = append(out, t)
		}
	}
	return out
}
