package pipeline

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// UnmarshalYAML decodes the `on` block. A trigger key with no value
// (`push:`) means "on, with defaults", which a plain pointer field would
// decode as off.
func (t *Triggers) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: on must be a mapping", n.Line)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i].Value, n.Content[i+1]
		isNull := val.Kind == yaml.ScalarNode && val.Tag == "!!null"
		var err error
		switch key {
		case "push":
			t.Push = &RefTrigger{}
			err = decodeUnlessNull(val, isNull, t.Push)
		case "pull_request":
			t.PullRequest = &PRTrigger{}
			err = decodeUnlessNull(val, isNull, t.PullRequest)
		case "tag":
			t.Tag = &TagTrigger{}
			err = decodeUnlessNull(val, isNull, t.Tag)
		case "manual":
			t.Manual = &ManualTrigger{}
			err = decodeUnlessNull(val, isNull, t.Manual)
		case "schedule":
			err = val.Decode(&t.Schedule)
		case "api":
			err = val.Decode(&t.API)
		default:
			err = fmt.Errorf("line %d: unknown trigger %q", n.Content[i].Line, key)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func decodeUnlessNull(val *yaml.Node, isNull bool, out any) error {
	if isNull {
		return nil
	}
	return val.Decode(out)
}

// IsEmpty reports whether no trigger is configured.
func (t Triggers) IsEmpty() bool {
	return t.Push == nil && t.PullRequest == nil && t.Tag == nil && t.Manual == nil && len(t.Schedule) == 0 && !t.API
}
