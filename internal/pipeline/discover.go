package pipeline

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DiscoverDirs returns the candidate pipeline directories relative to a
// repository root: a generic name first, then the branded one, so a
// product rename never strands existing repositories. brandedName is the
// caller's brand short name; pass "" to skip it.
func DiscoverDirs(brandedName string) []string {
	dirs := []string{".pipelines", filepath.Join(".ci", "pipelines")}
	if brandedName != "" {
		dirs = append(dirs, filepath.Join("."+brandedName, "pipelines"))
	}
	return dirs
}

// Discover returns the pipeline files under repoDir (*.yaml and *.yml in
// the first candidate directory that exists), sorted by name.
func Discover(repoDir, brandedName string) ([]string, error) {
	for _, d := range DiscoverDirs(brandedName) {
		entries, err := os.ReadDir(filepath.Join(repoDir, d))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("pipeline: read %s: %w", d, err)
		}
		var files []string
		for _, e := range entries {
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if !e.IsDir() && (ext == ".yaml" || ext == ".yml") {
				files = append(files, filepath.Join(repoDir, d, e.Name()))
			}
		}
		sort.Strings(files)
		return files, nil
	}
	return nil, nil
}
