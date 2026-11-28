package spec

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ScaffoldOptions picks the values a starter app.yaml is filled with.
type ScaffoldOptions struct {
	Name   string // service name
	Type   string // starter kind: web (default), worker or static
	Image  string // prebuilt image; empty means build from a Dockerfile
	Build  string // build type when Image is empty: dockerfile, railpack or static
	Port   int
	Domain string
	DB     string // optional database engine
}

// defaultScaffoldPort is the conventional web port the spec requires for non-static services.
const defaultScaffoldPort = 3000

var scaffoldName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

type scaffoldSchema struct {
	Defs map[string]struct {
		Properties map[string]struct {
			Description string `json:"description"`
			Enum        []any  `json:"enum"`
			Type        any    `json:"type"`
			Default     any    `json:"default"`
			Ref         string `json:"$ref"`
		} `json:"properties"`
	} `json:"$defs"`
}

// Scaffold renders a starter app.yaml. Optional field names and hints come
// from the embedded schema so the file cannot drift from what Parse accepts.
func Scaffold(o ScaffoldOptions) ([]byte, error) {
	if o.Name == "" {
		o.Name = "web"
	}
	if !scaffoldName.MatchString(o.Name) {
		return nil, fmt.Errorf("name %q must be lowercase letters, digits and dashes", o.Name)
	}
	if o.Port < 0 || o.Port > 65535 {
		return nil, fmt.Errorf("port %d is out of range", o.Port)
	}
	switch o.Type {
	case "", "web":
	case "worker":
		if o.Domain != "" {
			return nil, fmt.Errorf("type worker serves no public domain, drop --domain or use type web")
		}
	case "static":
		if o.Image != "" || (o.Build != "" && o.Build != "static") {
			return nil, fmt.Errorf("type static builds a site, it cannot take an image or another build type")
		}
		o.Build = "static"
	default:
		return nil, fmt.Errorf("type %q must be web, worker or static", o.Type)
	}
	raw, err := SchemaJSON()
	if err != nil {
		return nil, err
	}
	var sc scaffoldSchema
	if err := json.Unmarshal(raw, &sc); err != nil {
		return nil, fmt.Errorf("spec: read embedded schema: %w", err)
	}
	build := o.Build
	if build == "" {
		build = "dockerfile"
	}
	if o.Image == "" {
		if !schemaHasEnum(sc, "build", "type", build) || build == "image" || build == "compose" {
			return nil, fmt.Errorf("build %q is not a supported starter type (dockerfile, railpack, static)", build)
		}
	}
	if o.DB != "" && !schemaHasEnum(sc, "database", "engine", o.DB) {
		return nil, fmt.Errorf("database engine %q is not in the schema", o.DB)
	}

	if o.Port == 0 && build != "static" {
		o.Port = defaultScaffoldPort
	}
	var b strings.Builder
	b.WriteString("# Levelrail app spec. Validate with: levelrail-cli apps validate --file app.yaml\n")
	b.WriteString("version: 1\nservices:\n")
	fmt.Fprintf(&b, "  %s:\n", o.Name)
	if o.Image != "" {
		fmt.Fprintf(&b, "    image: %s\n", yamlScalar(o.Image))
	} else {
		fmt.Fprintf(&b, "    build:\n      type: %s\n", build)
	}
	if o.Port > 0 {
		fmt.Fprintf(&b, "    port: %d\n", o.Port)
	}
	if o.Domain != "" {
		fmt.Fprintf(&b, "    domains:\n      - %s\n", yamlScalar(o.Domain))
	}
	if o.Type == "worker" {
		b.WriteString("    # Background worker: no domain. Set command below to override the image entrypoint.\n")
	}
	b.WriteString(commentedFields(sc, "service", "    ", "Optional service fields (uncomment and edit):",
		"build", "image", "port", "domains"))
	if o.DB != "" {
		fmt.Fprintf(&b, "databases:\n  db:\n    engine: %s\n", o.DB)
		b.WriteString(commentedFields(sc, "database", "    ", "Optional database fields:", "engine"))
	}
	out := []byte(b.String())
	if _, err := Parse(out); err != nil {
		return nil, fmt.Errorf("spec: generated scaffold does not validate: %w", err)
	}
	return out, nil
}

func schemaHasEnum(sc scaffoldSchema, def, prop, val string) bool {
	for _, e := range sc.Defs[def].Properties[prop].Enum {
		if s, ok := e.(string); ok && s == val {
			return true
		}
	}
	return false
}

// fieldHint renders "(type, default X)" from schema facts only.
func fieldHint(typ any, ref string, def any) string {
	var parts []string
	if t, ok := typ.(string); ok {
		parts = append(parts, t)
	} else if ref != "" {
		parts = append(parts, "object")
	}
	if def != nil {
		parts = append(parts, fmt.Sprintf("default %v", def))
	}
	if len(parts) == 0 {
		return ""
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// optionalFieldComments renders the commented field list for one schema definition.
func optionalFieldComments(raw []byte, def, indent, heading string, skip ...string) (string, error) {
	var sc scaffoldSchema
	if err := json.Unmarshal(raw, &sc); err != nil {
		return "", fmt.Errorf("spec: read schema: %w", err)
	}
	return commentedFields(sc, def, indent, heading, skip...), nil
}

func commentedFields(sc scaffoldSchema, def, indent, heading string, skip ...string) string {
	skipped := map[string]bool{}
	for _, s := range skip {
		skipped[s] = true
	}
	names := make([]string, 0, len(sc.Defs[def].Properties))
	for n := range sc.Defs[def].Properties {
		if !skipped[n] {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "%s# %s\n", indent, heading)
	for _, n := range names {
		p := sc.Defs[def].Properties[n]
		desc := strings.TrimSpace(p.Description)
		if hint := fieldHint(p.Type, p.Ref, p.Default); hint != "" {
			desc = strings.TrimSpace(desc + " " + hint)
		}
		if desc == "" {
			fmt.Fprintf(&b, "%s# %s:\n", indent, n)
			continue
		}
		fmt.Fprintf(&b, "%s# %s: %s\n", indent, n, desc)
	}
	return b.String()
}

// yamlScalar quotes values that YAML would otherwise read as something else.
func yamlScalar(s string) string {
	if strings.ContainsAny(s, ":#{}[],&*!|>'\"%@`") && !strings.HasPrefix(s, "\"") {
		return fmt.Sprintf("%q", s)
	}
	return s
}
