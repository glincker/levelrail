package functions

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// DevSpec is how to run a handler directory in a throwaway container.
type DevSpec struct {
	Runtime string
	Image   string
	Command []string
}

var devImages = map[string]string{
	"node":   "node:lts-alpine",
	"python": "python:3-slim",
	"go":     "golang:alpine",
}

var devCommands = map[string][]string{
	"node":   {"npm", "start"},
	"python": {"sh", "-c", "pip install --no-cache-dir -r requirements.txt && python main.py"},
	"go":     {"go", "run", "."},
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

// DockerRunArgs builds the docker run arguments: the handler mounted at /app, PORT set last so it wins over the env file.
func (s DevSpec) DockerRunArgs(absDir, envFile string, port int) []string {
	p := strconv.Itoa(port)
	args := []string{"run", "--rm", "-i", "-p", p + ":" + p, "-v", absDir + ":/app", "-w", "/app"}
	if envFile != "" {
		args = append(args, "--env-file", envFile)
	}
	args = append(args, "-e", "PORT="+p, s.Image)
	return append(args, s.Command...)
}
