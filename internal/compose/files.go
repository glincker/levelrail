package compose

import (
	"fmt"
	"path"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// rawConfig is a top-level configs: entry. Only an inline content: body is
// translated; file:, external: and environment: sources stay unsupported
// because there is no checkout or host path to read them from.
type rawConfig struct {
	Content string `yaml:"content"`
}

// resolveConfigFiles turns service configs: references to inline top-level
// configs into Service.Files and drops the references it handled, so only
// shapes with no translation reach the unsupported check.
func resolveConfigFiles(f *File, top map[string]rawConfig) error {
	for name, svc := range f.Services {
		var left []any
		for _, ref := range svc.Configs {
			source, target, mode := configRef(ref)
			cfg, ok := top[source]
			if !ok || cfg.Content == "" {
				left = append(left, ref)
				continue
			}
			if target == "" {
				target = "/" + source
			}
			if err := spec.ValidateFilePath(target); err != nil {
				return fmt.Errorf("compose: service %q: configs %q: %w", name, source, err)
			}
			svc.Files = append(svc.Files, spec.File{Path: path.Clean(target), Content: cfg.Content, Mode: mode})
		}
		svc.Configs = left
		f.Services[name] = svc
	}
	for _, cfg := range top {
		if cfg.Content == "" {
			f.HasConfigs = true
			return nil
		}
	}
	f.HasConfigs = false
	return nil
}

// configRef reads the short ("name") and long ({source, target, mode})
// forms of a service configs: entry.
func configRef(ref any) (source, target, mode string) {
	switch v := ref.(type) {
	case string:
		return v, "", ""
	case map[string]any:
		source, _ = v["source"].(string)
		target, _ = v["target"].(string)
		switch m := v["mode"].(type) {
		case int:
			mode = fmt.Sprintf("%04o", m)
		case string:
			mode = m
		}
	}
	return source, target, mode
}
