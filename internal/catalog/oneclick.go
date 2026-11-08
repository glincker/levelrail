package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// MaxOneClickBytes caps a one-click app document so a pasted file cannot buffer without limit.
const MaxOneClickBytes = 256 << 10

// OneClickResult is a custom template draft built from a one-click app document.
type OneClickResult struct {
	Name        string
	Description string
	ComposeYAML string
	// Variables are the ${NAME} placeholders the operator fills in at deploy.
	Variables []string
	// Warnings list every key that was dropped or rewritten.
	Warnings []string
}

type oneClickDoc struct {
	Services map[string]oneClickService `json:"services"`
}

// oneClickMeta is the block under the vendor-prefixed "<prefix>OneClickApp" key.
type oneClickMeta struct {
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Variables   []struct {
		ID           string `json:"id"`
		DefaultValue string `json:"defaultValue"`
	} `json:"variables"`
}

type oneClickService struct {
	Image       string            `json:"image"`
	Command     any               `json:"command"`
	Environment map[string]string `json:"environment"`
	Volumes     []string          `json:"volumes"`
	Ports       []string          `json:"ports"`
	DependsOn   []string          `json:"depends_on"`
	Restart     string            `json:"restart"`
}

var (
	oneClickVarRe  = regexp.MustCompile(`\$\$cap_([A-Za-z0-9_]+)`)
	oneClickNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
)

// supportedServiceKeys are the service keys the converter understands; anything else is reported.
var supportedServiceKeys = map[string]bool{
	"image": true, "command": true, "environment": true, "volumes": true,
	"ports": true, "depends_on": true, "restart": true,
}

// ConvertOneClick turns a one-click app JSON document into a custom template draft.
// Unsupported keys are dropped with a warning; the result only ever becomes a custom template.
func ConvertOneClick(doc []byte) (OneClickResult, error) {
	if len(doc) > MaxOneClickBytes {
		return OneClickResult{}, fmt.Errorf("one-click document is larger than %d KiB", MaxOneClickBytes>>10)
	}
	var d oneClickDoc
	if err := json.Unmarshal(doc, &d); err != nil {
		return OneClickResult{}, fmt.Errorf("one-click document is not valid JSON: %w", err)
	}
	if len(d.Services) == 0 {
		return OneClickResult{}, errors.New("one-click document has no services")
	}
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(doc, &raw)

	// The metadata key carries a vendor prefix, so match on its fixed suffix.
	var meta oneClickMeta
	metaKey := ""
	for k, v := range raw {
		if strings.HasSuffix(k, "OneClickApp") {
			metaKey = k
			_ = json.Unmarshal(v, &meta)
		}
	}
	res := OneClickResult{Name: meta.DisplayName, Description: meta.Description}
	vars := map[string]bool{}
	defaults := map[string]string{}
	for _, v := range meta.Variables {
		defaults[v.ID] = v.DefaultValue
	}
	for k := range raw {
		if k != "services" && k != metaKey && k != "captainVersion" {
			res.Warnings = append(res.Warnings, fmt.Sprintf("top-level key %q dropped", k))
		}
	}

	names := make([]string, 0, len(d.Services))
	for n := range d.Services {
		names = append(names, n)
	}
	sort.Strings(names)

	services := yaml.Node{Kind: yaml.MappingNode}
	var svcRaw map[string]map[string]json.RawMessage
	if m, ok := raw["services"]; ok {
		_ = json.Unmarshal(m, &svcRaw)
	}
	rewrite := func(s string) string {
		return oneClickVarRe.ReplaceAllStringFunc(s, func(m string) string {
			id := m[len("$$cap_"):]
			vars[id] = true
			return "${" + strings.ToUpper(id) + "}"
		})
	}
	for _, n := range names {
		if !oneClickNameRe.MatchString(n) {
			return OneClickResult{}, fmt.Errorf("service name %q is not a valid compose service name", n)
		}
		s := d.Services[n]
		if strings.TrimSpace(s.Image) == "" {
			return OneClickResult{}, fmt.Errorf("service %q has no image", n)
		}
		for k := range svcRaw[n] {
			if !supportedServiceKeys[k] {
				res.Warnings = append(res.Warnings, fmt.Sprintf("service %q: key %q dropped", n, k))
			}
		}
		body := map[string]any{"image": rewrite(s.Image)}
		if s.Command != nil {
			body["command"] = s.Command
		}
		if len(s.Environment) > 0 {
			env := map[string]string{}
			for k, v := range s.Environment {
				env[k] = rewrite(v)
			}
			body["environment"] = env
		}
		if len(s.Volumes) > 0 {
			vs := make([]string, len(s.Volumes))
			for i, v := range s.Volumes {
				vs[i] = rewrite(v)
			}
			body["volumes"] = vs
		}
		if len(s.Ports) > 0 {
			body["ports"] = s.Ports
		}
		if len(s.DependsOn) > 0 {
			body["depends_on"] = s.DependsOn
		}
		if s.Restart != "" {
			body["restart"] = s.Restart
		}
		var key, val yaml.Node
		if err := key.Encode(n); err != nil {
			return OneClickResult{}, err
		}
		if err := val.Encode(body); err != nil {
			return OneClickResult{}, err
		}
		services.Content = append(services.Content, &key, &val)
	}

	root := yaml.Node{Kind: yaml.MappingNode}
	var sk yaml.Node
	_ = sk.Encode("services")
	root.Content = []*yaml.Node{&sk, &services}
	out, err := yaml.Marshal(&root)
	if err != nil {
		return OneClickResult{}, fmt.Errorf("render compose: %w", err)
	}
	res.ComposeYAML = string(out)
	for id := range vars {
		res.Variables = append(res.Variables, strings.ToUpper(id))
		if _, ok := defaults[id]; !ok {
			res.Warnings = append(res.Warnings, fmt.Sprintf("variable %q has no declared default", id))
		}
	}
	sort.Strings(res.Variables)
	sort.Strings(res.Warnings)
	return res, nil
}
