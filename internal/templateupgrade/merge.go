package templateupgrade

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// ErrUserServices means the installed file has services the template never
// had; applying the catalog body would delete them.
var ErrUserServices = errors.New("the app has services that are not part of the template")

// Merge returns the template body with the user's choices carried over: a
// pinned image tag, env values the plan marks keep_user, and env variables
// the user added. Everything else is the new template, so the result goes
// through the normal compose deploy path unchanged.
func Merge(installedBody, templateBody string, plan Plan) (string, error) {
	installed, err := compose.Parse([]byte(installedBody))
	if err != nil {
		return "", fmt.Errorf("templateupgrade: parse installed: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(templateBody), &doc); err != nil {
		return "", fmt.Errorf("templateupgrade: parse template: %w", err)
	}
	services := mapValue(docRoot(&doc), "services")
	if services == nil {
		return "", errors.New("templateupgrade: template has no services")
	}

	byName := map[string]ServicePlan{}
	for _, sp := range plan.Services {
		byName[sp.Name] = sp
	}
	present := map[string]bool{}
	for i := 0; i+1 < len(services.Content); i += 2 {
		name := services.Content[i].Value
		present[name] = true
		inst, ok := installed.Services[name]
		if !ok {
			continue
		}
		carryService(services.Content[i+1], inst, byName[name])
	}
	var user []string
	for name := range installed.Services {
		if !present[name] && byName[name].Status != ServiceRemoved {
			user = append(user, name)
		}
	}
	if len(user) > 0 {
		sort.Strings(user)
		return "", fmt.Errorf("%w: %s", ErrUserServices, strings.Join(user, ", "))
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return "", fmt.Errorf("templateupgrade: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		return "", fmt.Errorf("templateupgrade: encode: %w", err)
	}
	return buf.String(), nil
}

func carryService(node *yaml.Node, inst compose.Service, sp ServicePlan) {
	if sp.Image != nil && sp.Image.Preserved {
		if img := mapValue(node, "image"); img != nil {
			img.Value = inst.Image
		}
	}
	keep := map[string]bool{}
	removed := map[string]bool{}
	for _, e := range sp.Env {
		switch e.Action {
		case EnvKeepUser:
			keep[e.Name] = true
		case EnvRemove:
			removed[e.Name] = true
		}
	}
	tplEnv := envNames(node)
	names := make([]string, 0, len(inst.Environment))
	for n := range inst.Environment {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if keep[n] || (!tplEnv[n] && !removed[n]) {
			setEnv(node, n, inst.Environment[n])
		}
	}
}

func docRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return doc
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func envNames(svc *yaml.Node) map[string]bool {
	out := map[string]bool{}
	env := mapValue(svc, "environment")
	if env == nil {
		return out
	}
	switch env.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(env.Content); i += 2 {
			out[env.Content[i].Value] = true
		}
	case yaml.SequenceNode:
		for _, it := range env.Content {
			out[strings.SplitN(it.Value, "=", 2)[0]] = true
		}
	}
	return out
}

func setEnv(svc *yaml.Node, name, value string) {
	env := mapValue(svc, "environment")
	if env == nil {
		env = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		svc.Content = append(svc.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "environment"}, env)
	}
	switch env.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(env.Content); i += 2 {
			if env.Content[i].Value == name {
				env.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
				return
			}
		}
		env.Content = append(env.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
	case yaml.SequenceNode:
		line := name + "=" + value
		for _, it := range env.Content {
			if strings.SplitN(it.Value, "=", 2)[0] == name {
				it.Value = line
				return
			}
		}
		env.Content = append(env.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: line})
	}
}
