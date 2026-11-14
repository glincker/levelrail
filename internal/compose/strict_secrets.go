package compose

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// SecretMatcher reports whether a literal value under key must not live in a
// file. The IaC layer passes iac.LooksSecret so both guards share one detector.
type SecretMatcher func(key, value string) bool

// ScanSecretLiterals refuses plaintext secrets in environment, labels,
// command, entrypoint and healthcheck values. Findings name the file, line,
// service and key, never the value.
func ScanSecretLiterals(data []byte, file string, lim StrictLimits, match SecretMatcher) error {
	root, errs := strictRoot(data, lim)
	if len(errs) > 0 {
		return errs
	}
	services := resolveAlias(mappingValue(root, "services"))
	if services == nil || services.Kind != yaml.MappingNode {
		return nil
	}
	var out StrictErrors
	for i := 0; i+1 < len(services.Content); i += 2 {
		name, body := services.Content[i].Value, resolveAlias(services.Content[i+1])
		hit := func(key string, line int) {
			out = append(out, &StrictError{Code: CodeStrictSecretLiteral, Service: name, Key: key, Line: line,
				Message: fmt.Sprintf("%s: %s looks like a secret literal; bind it through spec.compose.env with a secretRef", file, key)})
		}
		for _, p := range mappingPairs(body, 0) {
			switch p.key {
			case "environment", "labels":
				for _, kv := range keyValues(p.val) {
					if literalLooksSecret(kv.key, kv.val, match) {
						hit(p.key+"."+kv.key, kv.line)
					}
				}
			case "command", "entrypoint":
				for _, s := range scalars(p.val) {
					if literalLooksSecret(p.key, s.Value, match) {
						hit(p.key, s.Line)
					}
				}
			case "healthcheck":
				for _, h := range mappingPairs(p.val, 0) {
					if h.key != "test" {
						continue
					}
					for _, s := range scalars(h.val) {
						if literalLooksSecret("healthcheck.test", s.Value, match) {
							hit("healthcheck.test", s.Line)
						}
					}
				}
			}
		}
	}
	return out.orNil()
}

// literalLooksSecret checks only the literal text: a ${VAR} part arrives from
// the declared env map, so only the value shape of the rest is judged.
func literalLooksSecret(key, value string, match SecretMatcher) bool {
	if !strings.Contains(value, "${") {
		return match(key, strings.ReplaceAll(value, "$$", "$"))
	}
	rest := stripInterpolations(value)
	if strings.TrimSpace(rest) == "" {
		return false
	}
	return match("", rest)
}

func stripInterpolations(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '$' {
			b.WriteByte('$')
			i += 2
			continue
		}
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '{' {
			end, _ := closingBrace(s, i+2)
			i = end + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

type nodePair struct {
	key string
	val *yaml.Node
}

// mappingPairs flattens a mapping, following "<<" merge keys a few levels deep.
func mappingPairs(m *yaml.Node, depth int) []nodePair {
	m = resolveAlias(m)
	if m == nil || m.Kind != yaml.MappingNode || depth > 8 {
		return nil
	}
	var out []nodePair
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i].Value, resolveAlias(m.Content[i+1])
		if k != "<<" {
			out = append(out, nodePair{k, v})
			continue
		}
		if v != nil && v.Kind == yaml.SequenceNode {
			for _, c := range v.Content {
				out = append(out, mappingPairs(c, depth+1)...)
			}
			continue
		}
		out = append(out, mappingPairs(v, depth+1)...)
	}
	return out
}

func mappingValue(m *yaml.Node, key string) *yaml.Node {
	for _, p := range mappingPairs(m, 0) {
		if p.key == key {
			return p.val
		}
	}
	return nil
}

type lineKV struct {
	key, val string
	line     int
}

// keyValues reads both the map form and the KEY=VALUE list form.
func keyValues(n *yaml.Node) []lineKV {
	n = resolveAlias(n)
	if n == nil {
		return nil
	}
	var out []lineKV
	switch n.Kind {
	case yaml.MappingNode:
		for _, p := range mappingPairs(n, 0) {
			if p.val != nil && p.val.Kind == yaml.ScalarNode {
				out = append(out, lineKV{p.key, p.val.Value, p.val.Line})
			}
		}
	case yaml.SequenceNode:
		for _, c := range n.Content {
			c = resolveAlias(c)
			if c == nil || c.Kind != yaml.ScalarNode {
				continue
			}
			k, v, ok := strings.Cut(c.Value, "=")
			if ok {
				out = append(out, lineKV{k, v, c.Line})
			}
		}
	}
	return out
}

func scalars(n *yaml.Node) []*yaml.Node {
	n = resolveAlias(n)
	if n == nil {
		return nil
	}
	if n.Kind == yaml.ScalarNode {
		return []*yaml.Node{n}
	}
	var out []*yaml.Node
	if n.Kind == yaml.SequenceNode {
		for _, c := range n.Content {
			if c = resolveAlias(c); c != nil && c.Kind == yaml.ScalarNode {
				out = append(out, c)
			}
		}
	}
	return out
}

// InterpolateDeclared expands ${VAR} forms in one value from env only. The
// host environment is never read. An unset plain ${VAR} is compose_var_missing.
func InterpolateDeclared(value string, env map[string]string) (string, error) {
	var missing []string
	out, err := interpolateValue(value, env, func(w string) {
		var name string
		if _, scanErr := fmt.Sscanf(w, unsetVarFormat, &name); scanErr == nil && name != "" {
			missing = append(missing, name)
		}
	})
	if err != nil {
		return "", &StrictError{Code: CodeStrictVarMissing, Message: err.Error()}
	}
	if len(missing) > 0 {
		return "", &StrictError{Code: CodeStrictVarMissing, Key: missing[0],
			Message: fmt.Sprintf("variable %s is not declared in spec.compose.env", strings.Join(missing, ", "))}
	}
	return unescapeDollar(out), nil
}

// CheckDeclaredVars reports every ${VAR} in the file that env does not
// declare. Plan passes names with placeholder values, so no secret is read.
func CheckDeclaredVars(data []byte, file string, lim StrictLimits, env map[string]string) error {
	root, errs := strictRoot(data, lim)
	if len(errs) > 0 {
		return errs
	}
	var out StrictErrors
	seen := map[*yaml.Node]bool{}
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		n = resolveAlias(n)
		if n == nil || seen[n] {
			return
		}
		seen[n] = true
		if n.Kind == yaml.ScalarNode && strings.Contains(n.Value, "${") {
			if _, err := InterpolateDeclared(n.Value, env); err != nil {
				var se *StrictError
				if errors.As(err, &se) {
					se.Line = n.Line
					se.Message = file + ": " + se.Message
					out = append(out, se)
				}
			}
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(root)
	return out.orNil()
}
