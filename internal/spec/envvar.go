package spec

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// EnvVar is one entry in a service's env block. app.yaml allows two
// shapes for the same field, per the app spec's own example, which
// shows both:
//
//	DATABASE_URL: { from: postgres.main.url }
//	API_KEY:      { secret: true, required: true }
//
// A plain string scalar is also accepted as shorthand for a literal
// value, since forcing every simple env var into object form would make
// the common case more verbose than the shape it's modeled on (Docker
// Compose's own env shorthand).
type EnvVar struct {
	// Value is a literal value, set when the YAML node was a plain
	// string scalar rather than a mapping.
	Value string
	// From references another resource's computed value, e.g.
	// "postgres.main.url". Mutually exclusive with Value and Secret.
	From string
	// Secret means the operator provides this at deploy time via
	// the platform's own envelope-encrypted secret storage, never
	// written to app.yaml or the git repo.
	Secret bool
	// Required, only meaningful alongside Secret: fail the deploy if no
	// value has been provided, rather than starting the container with
	// the variable unset.
	Required bool
}

// UnmarshalYAML implements the string-or-object union described on
// EnvVar, using yaml.v3's node-based Unmarshaler interface: the node's
// Kind tells a scalar apart from a mapping before deciding which shape
// to decode into, rather than trying one and falling back on error.
func (e *EnvVar) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		var asString string
		if err := node.Decode(&asString); err != nil {
			return fmt.Errorf("env value: %w", err)
		}
		*e = EnvVar{Value: asString}
		return nil
	}

	var asObject struct {
		From     string `yaml:"from"`
		Secret   bool   `yaml:"secret"`
		Required bool   `yaml:"required"`
	}
	if err := node.Decode(&asObject); err != nil {
		return fmt.Errorf("env value must be a string or an object with from/secret/required: %w", err)
	}

	*e = EnvVar{
		From:     asObject.From,
		Secret:   asObject.Secret,
		Required: asObject.Required,
	}
	return nil
}
