package functions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DevMountPath is where the handler directory is mounted in the dev container.
const DevMountPath = "/app"

// DevSpec is how to run a handler directory in a throwaway container.
type DevSpec struct {
	Runtime string
	Image   string
	Command string
}

var devImages = map[string]string{
	"node":   "node:lts-alpine",
	"python": "python:3-slim",
	"go":     "golang:alpine",
}

var devCommands = map[string]string{
	"node":   "npm start",
	"python": "pip install --no-cache-dir -r requirements.txt && python main.py",
	"go":     "go run .",
}

var devMarkers = []struct{ file, runtime string }{
	{"package.json", "node"},
	{"requirements.txt", "python"},
	{"main.py", "python"},
	{"go.mod", "go"},
}

// DetectDevSpec picks the runtime of dir from its manifest file. image overrides the default image.
func DetectDevSpec(dir, image string) (DevSpec, error) {
	for _, m := range devMarkers {
		if _, err := os.Stat(filepath.Join(dir, m.file)); err == nil {
			spec := DevSpec{Runtime: m.runtime, Image: devImages[m.runtime], Command: devCommands[m.runtime]}
			if image != "" {
				spec.Image = image
			}
			return spec, nil
		}
	}
	return DevSpec{}, fmt.Errorf("no package.json, requirements.txt, main.py or go.mod in %s", dir)
}

// ShellCommand runs the handler from the mounted directory; the Engine API has no working-dir field on ContainerSpec.
func (s DevSpec) ShellCommand() []string {
	return []string{"sh", "-c", "cd " + DevMountPath + " && " + s.Command}
}

// ParseEnvFile reads KEY=VALUE lines, skipping blanks and # comments.
func ParseEnvFile(data []byte) map[string]string {
	env := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			continue
		}
		env[k] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return env
}
