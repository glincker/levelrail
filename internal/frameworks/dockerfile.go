package frameworks

import "embed"

//go:embed dockerfiles/*.Dockerfile
var dockerfiles embed.FS

// DockerfileTemplate returns the tuned Dockerfile for presets that ship one.
func DockerfileTemplate(id string) (string, bool) {
	p, ok := Get(id)
	if !ok || !p.Dockerfile {
		return "", false
	}
	raw, err := dockerfiles.ReadFile("dockerfiles/" + id + ".Dockerfile")
	if err != nil {
		return "", false
	}
	return string(raw), true
}
