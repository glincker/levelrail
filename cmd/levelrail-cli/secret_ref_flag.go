package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/secretsource"
)

// secretRefFlag is NAME=secret://conn/path#key, repeatable. A literal value is
// rejected so --secret on env set can never carry a secret on the command line.
type secretRefFlag map[string]appVaultEnvRef

func (m secretRefFlag) String() string {
	pairs := make([]string, 0, len(m))
	for _, k := range sortedRefKeys(m) {
		v := m[k]
		pairs = append(pairs, k+"="+secretsource.Ref{Conn: v.Conn, Path: v.Path, Key: v.Key, Version: v.Version}.String())
	}
	return strings.Join(pairs, ",")
}

func (m secretRefFlag) Set(s string) error {
	name, ref, ok := strings.Cut(s, "=")
	if !ok || name == "" {
		return fmt.Errorf("--secret wants NAME=secret://conn/path#key")
	}
	// The reason never echoes the right side: a mistyped value may be a secret.
	if !secretsource.IsRef(ref) {
		return fmt.Errorf("--secret %s: the value must be a secret:// reference, not a literal", name)
	}
	parsed, err := secretsource.ParseRef(ref)
	if err != nil {
		return fmt.Errorf("--secret %s: %w", name, err)
	}
	m[name] = appVaultEnvRef{Conn: parsed.Conn, Path: parsed.Path, Key: parsed.Key, Version: parsed.Version}
	return nil
}

func sortedRefKeys(m map[string]appVaultEnvRef) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
