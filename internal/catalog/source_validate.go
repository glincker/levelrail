package catalog

import (
	"errors"
	"fmt"

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
	return errors.Join(errs...)
}
