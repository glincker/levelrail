package catalog

import (
	"fmt"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// checkTemplateFiles holds a rendered template's inline files to the same
// path and size rules an app.yaml file obeys, so a catalog entry cannot
// ship one a deploy would reject.
func checkTemplateFiles(services []store.DesiredService) error {
	for _, d := range services {
		files := make([]spec.File, len(d.Files))
		for i, f := range d.Files {
			files[i] = spec.File{Path: f.Path, Mode: f.Mode, Content: f.Content}
		}
		if err := spec.ValidateFiles(files, spec.FileLimitsFromEnv()); err != nil {
			return fmt.Errorf("service %q files: %w", d.Name, err)
		}
	}
	return nil
}
