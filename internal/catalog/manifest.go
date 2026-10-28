package catalog

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Generate kinds an env prompt may ask for, mapped onto the compose
// magic-var kinds that generate and persist a value per app.
var generateKinds = map[string]string{
	"password": "PASSWORD",
	"user":     "USER",
	"hex":      "HEX",
	"base64":   "BASE64",
}

// Manifest is one template directory: template.yaml plus the compose file it names.
type Manifest struct {
	ID                     string               `yaml:"id" json:"id"`
	Name                   string               `yaml:"name" json:"name"`
	Description            string               `yaml:"description" json:"description"`
	Icon                   string               `yaml:"icon" json:"icon,omitempty"`
	Category               string               `yaml:"category" json:"category"`
	DocumentationURL       string               `yaml:"documentation_url" json:"documentation_url"`
	ComposeFile            string               `yaml:"compose_file" json:"-"`
	RecommendedMemoryBytes int64                `yaml:"recommended_memory_bytes" json:"recommended_memory_bytes,omitempty"`
	Env                    []EnvPrompt          `yaml:"env" json:"env,omitempty"`
	Domains                []DomainPrompt       `yaml:"domains" json:"domains,omitempty"`
	Healthcheck            *ManifestHealthcheck `yaml:"healthcheck" json:"healthcheck,omitempty"`
	Compose                string               `yaml:"-" json:"-"`
}

// EnvPrompt is one ${KEY} placeholder the installer asks for.
type EnvPrompt struct {
	Key         string `yaml:"key" json:"key"`
	Label       string `yaml:"label" json:"label,omitempty"`
	Description string `yaml:"description" json:"description,omitempty"`
	Default     string `yaml:"default" json:"default,omitempty"`
	Generate    string `yaml:"generate" json:"generate,omitempty"`
	Length      int    `yaml:"length" json:"length,omitempty"`
	Required    bool   `yaml:"required" json:"required,omitempty"`
	Secret      bool   `yaml:"secret" json:"secret,omitempty"`
}

// DomainPrompt is a service the installer offers to put behind a domain.
type DomainPrompt struct {
	Service     string `yaml:"service" json:"service"`
	Port        int    `yaml:"port" json:"port"`
	Description string `yaml:"description" json:"description,omitempty"`
}

// ManifestHealthcheck is the HTTP check that proves an install actually serves.
type ManifestHealthcheck struct {
	Service string `yaml:"service" json:"service"`
	Port    int    `yaml:"port" json:"port"`
	Path    string `yaml:"path" json:"path"`
}

// InstallInput is what an operator supplies when installing a manifest.
type InstallInput struct {
	Env     map[string]string
	Domains map[string]string
}

var (
	placeholderPattern = regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*)\}`)
	envKeyPattern      = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	idPattern          = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	hostnamePattern    = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
)

// Render returns the manifest's compose file with every prompt filled in.
// Values are substituted into parsed YAML scalars, never into raw text, so
// an operator value cannot change the document's structure. With
// allowMissing, a required prompt with no value becomes an unresolved
// magic var, which the deploy path reports as "needs configuration".
func (m Manifest) Render(in InstallInput, allowMissing bool) (string, error) {
	values, err := m.promptValues(in.Env, allowMissing)
	if err != nil {
		return "", err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(m.Compose), &doc); err != nil {
		return "", fmt.Errorf("template %s: parse compose: %w", m.ID, err)
	}
	substitute(&doc, values)
	if err := m.addDomains(&doc, in.Domains); err != nil {
		return "", err
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return "", fmt.Errorf("template %s: render compose: %w", m.ID, err)
	}
	return string(out), nil
}

func (m Manifest) promptValues(supplied map[string]string, allowMissing bool) (map[string]string, error) {
	values := make(map[string]string, len(m.Env))
	var missing []string
	for _, p := range m.Env {
		v := supplied[p.Key]
		switch {
		case v != "":
		case p.Generate != "":
			v = generatedToken(p)
		case p.Default != "":
			v = p.Default
		case p.Required && allowMissing:
			v = "${SERVICE_PROMPT_" + p.Key + "}"
		case p.Required:
			missing = append(missing, p.Key)
		}
		values[p.Key] = v
	}
	for k := range supplied {
		if !m.hasPrompt(k) {
			return nil, fmt.Errorf("template %s has no setting %q", m.ID, k)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("template %s needs a value for: %s", m.ID, strings.Join(missing, ", "))
	}
	return values, nil
}

func generatedToken(p EnvPrompt) string {
	kind := generateKinds[p.Generate]
	if p.Length > 0 {
		return "${SERVICE_" + kind + "_" + strconv.Itoa(p.Length) + "_" + p.Key + "}"
	}
	return "${SERVICE_" + kind + "_" + p.Key + "}"
}

func (m Manifest) hasPrompt(key string) bool {
	for _, p := range m.Env {
		if p.Key == key {
			return true
		}
	}
	return false
}

func substitute(n *yaml.Node, values map[string]string) {
	if n.Kind == yaml.ScalarNode {
		n.Value = placeholderPattern.ReplaceAllStringFunc(n.Value, func(tok string) string {
			key := placeholderPattern.FindStringSubmatch(tok)[1]
			if v, ok := values[key]; ok {
				return v
			}
			return tok
		})
		return
	}
	for _, c := range n.Content {
		substitute(c, values)
	}
}

func (m Manifest) addDomains(doc *yaml.Node, domains map[string]string) error {
	if len(domains) == 0 {
		return nil
	}
	keys := make([]string, 0, len(domains))
	for svc, host := range domains {
		if !m.offersDomain(svc) {
			return fmt.Errorf("template %s does not offer a domain for service %q", m.ID, svc)
		}
		if !hostnamePattern.MatchString(host) {
			return fmt.Errorf("domain %q is not a valid hostname", host)
		}
		keys = append(keys, svc)
	}
	sort.Strings(keys)
	ext := &yaml.Node{Kind: yaml.MappingNode}
	for _, svc := range keys {
		ext.Content = append(ext.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: svc}, &yaml.Node{Kind: yaml.ScalarNode, Value: domains[svc]})
	}
	root := doc.Content[0]
	root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "x-levelrail-domains"}, ext)
	return nil
}

func (m Manifest) offersDomain(service string) bool {
	for _, d := range m.Domains {
		if d.Service == service {
			return true
		}
	}
	return false
}

// Template converts the manifest into a catalog entry; its Compose is the
// default render, so the generic one-click path keeps working.
func (m Manifest) Template() (Template, error) {
	body, err := m.Render(InstallInput{}, true)
	if err != nil {
		return Template{}, err
	}
	mm := m
	return Template{
		ID: m.ID, Name: m.Name, Slogan: m.Description, Description: m.Description, Category: m.Category, DocumentationURL: m.DocumentationURL,
		Compose: body, RecommendedMemoryBytes: m.RecommendedMemoryBytes, Manifest: &mm,
	}, nil
}
