package deploy

import (
	"io"
	"os"
	"path/filepath"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/defparse"
)

// definitionDockerfile returns the Dockerfile that levelrail-definition.json names
// in buildRoot, or "" when the file is absent or unusable. Webhook and manual
// builds share this path with git push deploys, so an unusable file never fails a build.
func definitionDockerfile(sourceDir, buildRoot string) string {
	defPath := filepath.Join(buildRoot, defparse.FileName)
	if build.EnsureWithin(sourceDir, defPath) != nil {
		return ""
	}
	f, err := os.Open(defPath) //nolint:gosec // inside the checkout, checked above
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, defparse.MaxBytes+1))
	if err != nil {
		return ""
	}
	def, err := defparse.Parse(b)
	if err != nil || def.ImageName != "" || len(def.DockerfileLines) > 0 || def.DockerfilePath == "" {
		return ""
	}
	full := filepath.Join(buildRoot, filepath.FromSlash(def.DockerfilePath))
	if build.EnsureWithin(sourceDir, full) != nil {
		return ""
	}
	if st, err := os.Stat(full); err != nil || !st.Mode().IsRegular() {
		return ""
	}
	return full
}
