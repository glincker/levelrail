package catalog

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// PromptTypeFile marks an env prompt that supplies a whole config file.
const PromptTypeFile = "file"

func (p EnvPrompt) isFile() bool { return p.Type == PromptTypeFile }

// FileInputKeys lists the prompt keys that carry file bodies, so callers keep
// them out of interpolation maps and logs.
func (m Manifest) FileInputKeys() []string {
	var keys []string
	for _, p := range m.Env {
		if p.isFile() {
			keys = append(keys, p.Key)
		}
	}
	return keys
}

// CheckFileInputs holds supplied file bodies to a byte cap set at runtime.
func (m Manifest) CheckFileInputs(supplied map[string]string, maxBytes int) error {
	for _, p := range m.Env {
		if p.isFile() && maxBytes > 0 && len(supplied[p.Key]) > maxBytes {
			return fmt.Errorf("setting %s is %d bytes, over the %d byte limit", p.Key, len(supplied[p.Key]), maxBytes)
		}
	}
	return nil
}

func (m Manifest) validateFilePrompts() []error {
	var errs []error
	seen := map[string]bool{}
	for _, p := range m.Env {
		if !p.isFile() {
			if p.Type != "" || p.Service != "" || p.Path != "" || p.Mode != "" {
				errs = append(errs, fmt.Errorf("env %s: type, service, path and mode apply only to type file", p.Key))
			}
			continue
		}
		switch {
		case p.Generate != "":
			errs = append(errs, fmt.Errorf("env %s: a file input cannot use generate; put a generated ${KEY} inside its default body", p.Key))
		case p.Service == "":
			errs = append(errs, fmt.Errorf("env %s: a file input needs a service", p.Key))
		}
		if err := spec.ValidateFilePath(p.Path); err != nil {
			errs = append(errs, fmt.Errorf("env %s: %w", p.Key, err))
		}
		if _, err := spec.ParseFileMode(p.Mode); err != nil {
			errs = append(errs, fmt.Errorf("env %s: %w", p.Key, err))
		}
		if id := p.Service + " " + p.Path; seen[id] {
			errs = append(errs, fmt.Errorf("env %s: path %s is mounted twice in %s", p.Key, p.Path, p.Service))
		} else {
			seen[id] = true
		}
	}
	return errs
}

// addFiles turns each file prompt into a compose configs: entry, or a
// secrets: entry when the prompt is secret, so the engine's own file
// translation, size checks and redaction apply. A supplied body is literal;
// only the author's default body expands ${KEY} prompts.
func (m Manifest) addFiles(doc *yaml.Node, supplied, values map[string]string, allowMissing bool) error {
	root := doc.Content[0]
	for _, p := range m.Env {
		if !p.isFile() {
			continue
		}
		body := supplied[p.Key]
		if body == "" {
			body = expandPlaceholders(p.Default, values)
		}
		if body == "" {
			switch {
			case p.Required && allowMissing:
				body = "${SERVICE_PROMPT_" + p.Key + "}"
			case p.Required:
				return fmt.Errorf("template %s needs a value for: %s", m.ID, p.Key)
			default:
				continue
			}
		}
		svc := mapValue(mapValue(root, "services"), p.Service)
		if svc == nil {
			return fmt.Errorf("template %s: file input %s names service %q, which the compose file does not declare", m.ID, p.Key, p.Service)
		}
		kind, mode := "configs", p.Mode
		if p.Secret {
			kind = "secrets"
		}
		if mode == "" {
			mode = map[bool]string{true: "0400", false: "0444"}[p.Secret]
		}
		name := "lr_file_" + strings.ToLower(p.Key)
		entry := &yaml.Node{Kind: yaml.MappingNode}
		setScalar(entry, "content", body, yaml.LiteralStyle)
		setMapValue(ensureMap(root, kind), name, entry)
		ref := &yaml.Node{Kind: yaml.MappingNode}
		setScalar(ref, "source", name, 0)
		setScalar(ref, "target", p.Path, 0)
		setScalar(ref, "mode", mode, yaml.DoubleQuotedStyle)
		list := ensureSeq(svc, kind)
		list.Content = append(list.Content, ref)
	}
	return nil
}

func mapValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func setMapValue(n *yaml.Node, key string, val *yaml.Node) {
	n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, val)
}

func setScalar(n *yaml.Node, key, val string, style yaml.Style) {
	if style == yaml.LiteralStyle && !strings.Contains(val, "\n") {
		style = 0
	}
	setMapValue(n, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: val, Style: style})
}

func ensureMap(parent *yaml.Node, key string) *yaml.Node {
	if v := mapValue(parent, key); v != nil {
		return v
	}
	v := &yaml.Node{Kind: yaml.MappingNode}
	setMapValue(parent, key, v)
	return v
}

func ensureSeq(parent *yaml.Node, key string) *yaml.Node {
	if v := mapValue(parent, key); v != nil {
		return v
	}
	v := &yaml.Node{Kind: yaml.SequenceNode}
	setMapValue(parent, key, v)
	return v
}
