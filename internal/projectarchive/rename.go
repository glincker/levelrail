package projectarchive

import (
	"bytes"
	"errors"
	"io"

	"gopkg.in/yaml.v3"
)

// RenameProject rewrites the project name in every spec file: the Project
// document itself and the spec.project reference of the rest.
func RenameProject(files map[string][]byte, from, to string) (map[string][]byte, error) {
	out := make(map[string][]byte, len(files))
	for name, raw := range files {
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		for {
			var doc yaml.Node
			err := dec.Decode(&doc)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			renameDoc(&doc, from, to)
			if err := enc.Encode(&doc); err != nil {
				return nil, err
			}
		}
		if err := enc.Close(); err != nil {
			return nil, err
		}
		out[name] = buf.Bytes()
	}
	return out, nil
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

func renameDoc(doc *yaml.Node, from, to string) {
	root := doc
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	if k := mapValue(root, "kind"); k != nil && k.Value == "Project" {
		if name := mapValue(mapValue(root, "metadata"), "name"); name != nil && name.Value == from {
			name.Value = to
		}
		return
	}
	if p := mapValue(mapValue(root, "spec"), "project"); p != nil && p.Value == from {
		p.Value = to
	}
}
