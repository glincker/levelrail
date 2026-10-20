package spec

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DiscoverPath finds the app spec file in dir, checking candidate
// filenames in order and returning the first one that exists. The spec
// file is discovered by a list of candidate filenames (app.yaml,
// deploy.yaml, plus the branded one) so a product rename does not break
// existing users later. The generic names are checked first for exactly
// that reason: if the product is ever renamed, brandedName changes, but
// a repo with an app.yaml already committed keeps working without the
// user touching anything.
//
// brandedName is the caller-supplied branded filename stem (derived from
// brand.Brand, not imported here, so this package stays decoupled from
// internal/brand); pass "" to skip it, e.g. in tests.
func DiscoverPath(dir string, brandedName string) (string, error) {
	candidates := candidateFilenames(brandedName)
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		_, err := os.Stat(path)
		if err == nil {
			return path, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("spec: checking %s: %w", path, err)
		}
	}
	return "", fmt.Errorf("spec: no app spec found in %s, checked %v", dir, candidates)
}

func candidateFilenames(brandedName string) []string {
	names := []string{"app.yaml", "app.yml", "deploy.yaml", "deploy.yml"}
	if brandedName != "" {
		names = append(names, brandedName+".yaml", brandedName+".yml")
	}
	return names
}
