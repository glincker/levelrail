package spec

import (
	"fmt"
	"strconv"
	"strings"

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
	// "postgres.main.url". Mutually exclusive with Value, Secret, and
	// Vault.
	From string
	// Secret means the operator provides this at deploy time via
	// the platform's own envelope-encrypted secret storage, never
	// written to app.yaml or the git repo. Mutually exclusive with
	// Vault: a given env var resolves its value from exactly one of
	// the two secret sources, never both.
	Secret bool
	// Required, only meaningful alongside Secret: fail the deploy if no
	// value has been provided, rather than starting the container with
	// the variable unset.
	Required bool
	// Vault means this value is resolved live from an external
	// HashiCorp Vault instance at container-create time
	// (internal/reconcile/application's resolveEnv), never stored by
	// this platform at all: the alternative to Secret above, not a
	// variant of it. nil means not vault-backed.
	Vault *VaultRef
	// SecretRef points at a value in a named secret connection, written
	// { secret: { conn, path, key } } or the string secret://conn/path#key.
	// Resolved at container create, never stored as a value. Mutually
	// exclusive with every other source.
	SecretRef *SecretRef
}

// SecretRef is the unvalidated shape of a connection reference. Text holds the
// secret:// string form; otherwise Conn, Path, Key and Version hold the object
// form. internal/deploy validates it, so this package stays free of the resolver.
type SecretRef struct {
	Text    string
	Conn    string
	Path    string
	Key     string
	Version int
}

// VaultRef is app.yaml's { vault: { path, key } } env var shape: Path is
// the Vault KV secret's path, Key is the field name within that
// secret's data. Both required together, mirroring internal/store's own
// VaultEnvRef which this parses into (see internal/deploy's
// vaultEnvRefs).
type VaultRef struct {
	Path string `yaml:"path"`
	Key  string `yaml:"key"`
}

// Parts splits the reference without checking character rules, which the
// resolver enforces at deploy time (a bad reference fails closed there).
func (r SecretRef) Parts() (conn, path, key string, version int, err error) {
	if r.Text == "" {
		return r.Conn, r.Path, r.Key, r.Version, nil
	}
	rest := strings.TrimPrefix(r.Text, "secret://")
	loc, frag, ok := strings.Cut(rest, "#")
	conn, path, ok2 := strings.Cut(loc, "/")
	if !ok || !ok2 || conn == "" || path == "" {
		return "", "", "", 0, fmt.Errorf("secret reference must be secret://conn/path#key")
	}
	key, query, hasQuery := strings.Cut(frag, "?")
	if hasQuery {
		n, perr := strconv.Atoi(strings.TrimPrefix(query, "version="))
		if perr != nil || !strings.HasPrefix(query, "version=") || n < 1 {
			return "", "", "", 0, fmt.Errorf("secret reference query must be version=N")
		}
		version = n
	}
	if key == "" {
		return "", "", "", 0, fmt.Errorf("secret reference needs a key after #")
	}
	return conn, path, key, version, nil
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
		if strings.HasPrefix(asString, "secret://") {
			*e = EnvVar{SecretRef: &SecretRef{Text: asString}}
			return nil
		}
		*e = EnvVar{Value: asString}
		return nil
	}

	var asObject struct {
		From     string    `yaml:"from"`
		Secret   yaml.Node `yaml:"secret"`
		Required bool      `yaml:"required"`
		Vault    *VaultRef `yaml:"vault"`
	}
	if err := node.Decode(&asObject); err != nil {
		return fmt.Errorf("env value must be a string or an object with from/secret/required/vault: %w", err)
	}
	var secretFlag bool
	var secretRef *SecretRef
	switch asObject.Secret.Kind {
	case 0:
	case yaml.MappingNode:
		var rs struct {
			Conn    string `yaml:"conn"`
			Path    string `yaml:"path"`
			Key     string `yaml:"key"`
			Version int    `yaml:"version"`
		}
		if err := asObject.Secret.Decode(&rs); err != nil {
			return fmt.Errorf("env value: secret reference: %w", err)
		}
		if rs.Conn == "" || rs.Path == "" || rs.Key == "" {
			return fmt.Errorf("env value: secret reference needs conn, path and key")
		}
		secretRef = &SecretRef{Conn: rs.Conn, Path: rs.Path, Key: rs.Key, Version: rs.Version}
	default:
		if err := asObject.Secret.Decode(&secretFlag); err != nil {
			return fmt.Errorf("env value: secret must be true or a { conn, path, key } reference")
		}
	}
	if secretRef != nil {
		if asObject.Vault != nil || asObject.From != "" {
			return fmt.Errorf("env value: a secret reference is mutually exclusive with from and vault")
		}
		*e = EnvVar{Required: asObject.Required, SecretRef: secretRef}
		return nil
	}
	if asObject.Vault != nil && secretFlag {
		return fmt.Errorf("env value: vault and secret are mutually exclusive")
	}
	if asObject.Vault != nil && asObject.From != "" {
		return fmt.Errorf("env value: vault and from are mutually exclusive")
	}
	if asObject.Vault != nil && (asObject.Vault.Path == "" || asObject.Vault.Key == "") {
		return fmt.Errorf("env value: vault.path and vault.key are both required")
	}

	*e = EnvVar{
		From:     asObject.From,
		Secret:   secretFlag,
		Required: asObject.Required,
		Vault:    asObject.Vault,
	}
	return nil
}
