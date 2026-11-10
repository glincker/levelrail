package build

import (
	"os"
	"path/filepath"
	"strings"
)

// legacyBuildpackMarkers are fixed names only, never globbed or read.
var legacyBuildpackMarkers = []string{".buildpacks", "project.toml", "Procfile"}

// markerPresent uses Lstat so a symlinked marker is never followed or reported.
func markerPresent(dir, name string) bool {
	fi, err := os.Lstat(filepath.Join(dir, name))
	return err == nil && fi.Mode().IsRegular()
}

// DockerfileNote is the first build log line when a Dockerfile is used.
func DockerfileNote(contextDir, dockerfilePath string) string {
	rel := "Dockerfile"
	if dockerfilePath != "" {
		if r, err := filepath.Rel(contextDir, dockerfilePath); err == nil && !strings.HasPrefix(r, "..") {
			rel = filepath.ToSlash(r)
		} else {
			rel = filepath.Base(dockerfilePath)
		}
	}
	return "builder: Dockerfile (" + rel + ")"
}

// BuilderNotes returns the log lines for a Railpack build of sourceDir. It
// only reports marker file names, never their contents.
func BuilderNotes(sourceDir string) []string {
	notes := []string{"builder: Railpack (no Dockerfile in the repository)"}
	var found []string
	for _, m := range legacyBuildpackMarkers {
		if markerPresent(sourceDir, m) {
			found = append(found, m)
		}
	}
	if len(found) > 0 {
		notes = append(notes, "builder: found "+strings.Join(found, ", ")+"; .buildpacks and project.toml are not run as buildpacks, Railpack detects the language, and a Procfile web line is only used to guess the port")
	}
	return notes
}
