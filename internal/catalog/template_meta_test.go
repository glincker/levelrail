package catalog

import (
	"strings"
	"testing"
)

// Built from code points so this file itself stays free of the characters.
var dashRunes = string(rune(0x2013)) + string(rune(0x2014))

func TestTemplateMetaIsValid(t *testing.T) {
	byID := map[string]Template{}
	for _, tpl := range Templates {
		byID[tpl.ID] = tpl
	}
	for id, m := range templateMeta {
		tpl, ok := byID[id]
		if !ok {
			t.Errorf("%s: meta for an unknown template", id)
			continue
		}
		if tpl.Manifest != nil {
			t.Errorf("%s: manifest templates keep metadata in template.yaml", id)
		}
		if strings.ContainsAny(m.Description, dashRunes) {
			t.Errorf("%s: description has a dash character", id)
		}
		if m.Status != "" && m.Status != StatusActive {
			if strings.TrimSpace(m.StatusReason) == "" {
				t.Errorf("%s: status %s needs a status_reason", id, m.Status)
			}
			if strings.ContainsAny(m.StatusReason, dashRunes) {
				t.Errorf("%s: status_reason has a dash character", id)
			}
		}
		if m.ReplacedBy != "" {
			if _, ok := byID[m.ReplacedBy]; !ok {
				t.Errorf("%s: replaced_by %q is not a template", id, m.ReplacedBy)
			}
		}
		if m.License == "" {
			continue
		}
		if !githubRepoPattern.MatchString(m.SourceRepo) {
			t.Errorf("%s: bad source_repo %q", id, m.SourceRepo)
		}
		if !httpsURLPattern.MatchString(m.LicenseURL) {
			t.Errorf("%s: bad license_url %q", id, m.LicenseURL)
		}
		for _, err := range validateLicense(m.License, m.LicenseClass) {
			t.Errorf("%s: %v", id, err)
		}
		if m.LicenseClass == "" {
			t.Errorf("%s: missing license_class", id)
		}
	}
}
