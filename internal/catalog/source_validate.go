package catalog

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// ValidateSource applies the registry validator plus the submission safety
// checks (no privileged capability class, no bind mounts, no fetch at start)
// to a template that came from a private source. Marketplace metadata such as
// license and pinned tags is not required of a team's own templates.
func (m Manifest) ValidateSource() error {
	errs := []error{m.Validate()}
	errs = append(errs, m.validateSubmissionCapabilities()...)
	f, err := compose.Parse([]byte(m.Compose))
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	for name, svc := range f.Services {
		for _, v := range svc.Volumes {
			if v.Name == "" {
				errs = append(errs, fmt.Errorf("service %s: bind mounts are not allowed in source templates, use a named volume", name))
			}
		}
	}
	errs = append(errs, m.checkFetch()...)
	errs = append(errs, volumeDriverErrors(m.Compose)...)
	return errors.Join(errs...)
}

// ManifestFromSource rebuilds a stored source entry and validates it again, so a
// row that was edited in the database cannot install anything a sync would refuse.
func ManifestFromSource(templateYAML, composeYAML string) (Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal([]byte(templateYAML), &m); err != nil {
		return Manifest{}, fmt.Errorf("catalog: source template: %w", err)
	}
	m.Compose = composeYAML
	if err := m.ValidateSource(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// volumeDriverErrors refuses a named volume that is really a host bind: the
// local driver with type none, a bind option or an absolute device path.
func volumeDriverErrors(body string) []error {
	var doc struct {
		Volumes map[string]struct {
			DriverOpts map[string]string `yaml:"driver_opts"`
		} `yaml:"volumes"`
	}
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		return nil
	}
	var errs []error
	for name, v := range doc.Volumes {
		o := v.DriverOpts
		if strings.EqualFold(o["type"], "none") || strings.Contains(strings.ToLower(o["o"]), "bind") || strings.HasPrefix(o["device"], "/") {
			errs = append(errs, fmt.Errorf("volume %s: driver_opts that bind a host path are not allowed in source templates", name))
		}
	}
	return errs
}
