package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/frameworks"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// applyFrameworkPreset fills the git-build flags a preset knows (port, build
// type) without overriding anything the operator set explicitly.
func applyFrameworkPreset(f *createFlags) (*frameworks.Preset, error) {
	if f.framework == "" {
		return nil, nil
	}
	if f.image != "" || f.file != "" {
		return nil, newValidationError("--framework applies to the git-build path and cannot be combined with --image or --file")
	}
	p, ok := frameworks.Get(f.framework)
	if !ok {
		ids := make([]string, 0, len(frameworks.List()))
		for _, c := range frameworks.List() {
			ids = append(ids, c.ID)
		}
		return nil, newValidationError("unknown --framework %q; choose one of: %s", f.framework, strings.Join(ids, ", "))
	}
	if p.BuildType == frameworks.BuildStatic {
		return nil, newValidationError("%s builds to a static site: declare build.type: static in app.yaml and use --file, or pick it in the dashboard", p.Name)
	}
	if f.port <= 0 {
		f.port = p.Port
	}
	if f.buildType == "" {
		f.buildType = p.BuildType
	}
	return &p, nil
}

func presetVolumes(p *frameworks.Preset) []spec.Volume {
	vols := make([]spec.Volume, 0, len(p.Volumes))
	for _, v := range p.Volumes {
		vols = append(vols, spec.Volume{Name: v.Name, Path: v.Path})
	}
	return vols
}

func printPresetEnvHints(out io.Writer, p *frameworks.Preset, provided map[string]string) {
	for _, e := range p.EnvHints {
		if !e.Required {
			continue
		}
		if _, ok := provided[e.Key]; ok {
			continue
		}
		_, _ = fmt.Fprintf(out, "note:     %s needs %s: %s (set it with --secret %s=...)\n", p.Name, e.Key, e.Description, e.Key)
	}
}
