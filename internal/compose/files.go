package compose

import (
	"fmt"
	"path"

	"gopkg.in/yaml.v3"

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

// RedactedSecretMarker replaces an inline secret body in stored compose
// history; the real value lives only in the secrets store.
const RedactedSecretMarker = //nolint:gosec // a placeholder, not a credential
"__levelrail_redacted__"

// resolveSecretFiles turns service secrets: references to inline top-level
// secrets into secret files under /run/secrets, delivered verbatim.
func resolveSecretFiles(f *File, top map[string]rawConfig) error {
	for name, svc := range f.Services {
		var left []any
		for _, ref := range svc.Secrets {
			source, target, mode := configRef(ref)
			sec, ok := top[source]
			if !ok || sec.Content == "" {
				left = append(left, ref)
				continue
			}
			if target == "" {
				target = "/run/secrets/" + source
			} else if !path.IsAbs(target) {
				target = "/run/secrets/" + target
			}
			if mode == "" {
				mode = "0400"
			}
			if err := spec.ValidateFilePath(target); err != nil {
				return fmt.Errorf("compose: service %q: secrets %q: %w", name, source, err)
			}
			svc.Files = append(svc.Files, spec.File{Path: path.Clean(target), Content: sec.Content, Mode: mode, Secret: true, NoExpand: true})
		}
		svc.Secrets = left
		f.Services[name] = svc
	}
	f.HasSecrets = false
	for _, sec := range top {
		if sec.Content == "" {
			f.HasSecrets = true
			return nil
		}
	}
	return nil
}

// RedactSecretContent blanks every inline top-level secrets: body so a
// stored revision never holds plaintext. It returns the body unchanged when
// nothing needed redacting.
func RedactSecretContent(body []byte) ([]byte, bool, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return nil, false, fmt.Errorf("compose: redact: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return body, false, nil
	}
	changed := false
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "secrets" || root.Content[i+1].Kind != yaml.MappingNode {
			continue
		}
		entries := root.Content[i+1].Content
		for j := 0; j+1 < len(entries); j += 2 {
			entry := entries[j+1]
			if entry.Kind != yaml.MappingNode {
				continue
			}
			for k := 0; k+1 < len(entry.Content); k += 2 {
				val := entry.Content[k+1]
				if entry.Content[k].Value == "content" && val.Value != "" && val.Value != RedactedSecretMarker {
					val.Value, val.Style, val.Tag = RedactedSecretMarker, 0, "!!str"
					changed = true
				}
			}
		}
	}
	if !changed {
		return body, false, nil
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, false, fmt.Errorf("compose: redact: %w", err)
	}
	return out, true, nil
}
