package k8simport

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrInput marks every refusal of the raw input (size, shape, bombs).
var ErrInput = errors.New("k8simport: input refused")

type meta struct {
	Name        string            `yaml:"name"`
	Namespace   string            `yaml:"namespace"`
	Annotations map[string]string `yaml:"annotations"`
}

// object is the envelope every Kubernetes document shares.
type object struct {
	APIVersion string            `yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Metadata   meta              `yaml:"metadata"`
	Spec       yaml.Node         `yaml:"spec"`
	Data       map[string]string `yaml:"data"`
	StringData map[string]string `yaml:"stringData"`
	Type       string            `yaml:"type"`
	Items      []yaml.Node       `yaml:"items"`
	// Index is the 1-based position of the document in the input.
	Index int `yaml:"-"`
}

func (o object) ns() string {
	if o.Metadata.Namespace == "" {
		return "default"
	}
	return o.Metadata.Namespace
}

func (o object) key() string { return o.ns() + "/" + o.Metadata.Name }

// checkNode bounds depth, alias chains and the alias-expanded node count.
func checkNode(root *yaml.Node, l Limits) error {
	total := 0
	var walk func(n *yaml.Node, depth, anchorDepth int) error
	walk = func(n *yaml.Node, depth, anchorDepth int) error {
		if depth > l.MaxDepth {
			return fmt.Errorf("%w: nesting deeper than %d", ErrInput, l.MaxDepth)
		}
		total++
		if total > l.MaxNodes {
			return fmt.Errorf("%w: more than %d nodes after alias expansion", ErrInput, l.MaxNodes)
		}
		if n.Kind == yaml.AliasNode {
			if anchorDepth+1 > l.MaxAnchorDepth {
				return fmt.Errorf("%w: anchors nested deeper than %d", ErrInput, l.MaxAnchorDepth)
			}
			return walk(n.Alias, depth+1, anchorDepth+1)
		}
		for _, c := range n.Content {
			if err := walk(c, depth+1, anchorDepth); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root, 0, 0)
}

// parseDocuments splits the input into objects, flattening List kinds.
// Documents that do not decode are returned as bad indexes.
func parseDocuments(data []byte, l Limits) ([]object, []badDoc, error) {
	if len(data) > l.MaxBytes {
		return nil, nil, fmt.Errorf("%w: %d bytes exceeds the %d byte limit (%s)", ErrInput, len(data), l.MaxBytes, EnvMaxBytes)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var objs []object
	var bad []badDoc
	docs := 0
	for {
		var node yaml.Node
		err := dec.Decode(&node)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("%w: invalid yaml: %s", ErrInput, strings.TrimPrefix(err.Error(), "yaml: "))
		}
		docs++
		if docs > l.MaxDocs {
			return nil, nil, fmt.Errorf("%w: more than %d documents (%s)", ErrInput, l.MaxDocs, EnvMaxDocs)
		}
		if err := checkNode(&node, l); err != nil {
			return nil, nil, err
		}
		root := &node
		if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
			root = root.Content[0]
		}
		if root.Kind != yaml.MappingNode {
			if root.Kind != 0 && (root.Kind != yaml.ScalarNode || root.Tag != "!!null") {
				bad = append(bad, badDoc{Index: docs, Reason: "document is not a mapping"})
			}
			continue
		}
		var o object
		if err := root.Decode(&o); err != nil {
			bad = append(bad, badDoc{Index: docs, Reason: "does not decode as a Kubernetes object: " + trimDecodeErr(err)})
			continue
		}
		o.Index = docs
		if strings.HasSuffix(o.Kind, "List") && len(o.Items) > 0 {
			for i := range o.Items {
				var it object
				if err := o.Items[i].Decode(&it); err != nil {
					bad = append(bad, badDoc{Index: docs, Reason: "list item does not decode: " + trimDecodeErr(err)})
					continue
				}
				it.Index = docs
				objs = append(objs, it)
				if len(objs) > l.MaxDocs {
					return nil, nil, fmt.Errorf("%w: more than %d objects (%s)", ErrInput, l.MaxDocs, EnvMaxDocs)
				}
			}
			continue
		}
		objs = append(objs, o)
	}
	return objs, bad, nil
}

type badDoc struct {
	Index  int
	Reason string
}

func trimDecodeErr(err error) string {
	msg := strings.TrimPrefix(err.Error(), "yaml: ")
	if i := strings.Index(msg, "\n"); i >= 0 {
		msg = msg[:i]
	}
	return msg
}

func mapKeys(n *yaml.Node) []string {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	var keys []string
	for i := 0; i+1 < len(n.Content); i += 2 {
		keys = append(keys, n.Content[i].Value)
	}
	return keys
}
