package importer

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing/fstest"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

type outService struct {
	Image       string            `yaml:"image"`
	Restart     string            `yaml:"restart,omitempty"`
	Command     any               `yaml:"command,omitempty"`
	Entrypoint  any               `yaml:"entrypoint,omitempty"`
	Ports       []string          `yaml:"ports,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	Volumes     []string          `yaml:"volumes,omitempty"`
	DependsOn   any               `yaml:"depends_on,omitempty"`
	Healthcheck map[string]any    `yaml:"healthcheck,omitempty"`
	Privileged  bool              `yaml:"privileged,omitempty"`
	NetworkMode string            `yaml:"network_mode,omitempty"`
	Pid         string            `yaml:"pid,omitempty"`
	Ipc         string            `yaml:"ipc,omitempty"`
	CapAdd      []string          `yaml:"cap_add,omitempty"`
	Devices     []string          `yaml:"devices,omitempty"`
	SecurityOpt []string          `yaml:"security_opt,omitempty"`
	VolumesFrom []string          `yaml:"volumes_from,omitempty"`
}

type outCompose struct {
	Services map[string]*outService `yaml:"services"`
}

type outTemplate struct {
	ID                     string                       `yaml:"id"`
	Name                   string                       `yaml:"name"`
	Description            string                       `yaml:"description"`
	Icon                   string                       `yaml:"icon"`
	Category               string                       `yaml:"category"`
	DocumentationURL       string                       `yaml:"documentation_url"`
	RecommendedMemoryBytes int64                        `yaml:"recommended_memory_bytes"`
	Source                 *catalog.ManifestSource      `yaml:"source,omitempty"`
	Images                 []catalog.ManifestImage      `yaml:"images,omitempty"`
	Env                    []outPrompt                  `yaml:"env,omitempty"`
	Domains                []catalog.DomainPrompt       `yaml:"domains,omitempty"`
	Healthcheck            *catalog.ManifestHealthcheck `yaml:"healthcheck,omitempty"`
}

type outPrompt struct {
	Key         string `yaml:"key"`
	Label       string `yaml:"label,omitempty"`
	Description string `yaml:"description,omitempty"`
	Default     string `yaml:"default,omitempty"`
	Generate    string `yaml:"generate,omitempty"`
	Length      int    `yaml:"length,omitempty"`
	Required    bool   `yaml:"required,omitempty"`
	Secret      bool   `yaml:"secret,omitempty"`
}

func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *conversion) finish(id string, opt Options, out Output) Output {
	fail := func(code, detail string) Output {
		out.Skip = &Skip{Code: code, Detail: detail}
		out.Notes = c.t.notes
		return out
	}
	dports, err := c.domainPorts()
	if err != nil {
		var se *skipError
		errors.As(err, &se)
		return fail(se.code, se.detail)
	}
	var domains []catalog.DomainPrompt
	var hc *catalog.ManifestHealthcheck
	domainNames := make([]string, 0, len(dports))
	for s := range dports {
		domainNames = append(domainNames, s)
	}
	sort.Strings(domainNames)
	for _, s := range domainNames {
		c.out[s].Ports = []string{strconv.Itoa(dports[s])}
		c.httpHealthcheck(s, dports[s])
		domains = append(domains, catalog.DomainPrompt{Service: s, Port: dports[s], Description: "Public address of " + s + "."})
	}
	if !c.anyHealthcheck() {
		return fail(ReasonNoHealthcheck, "no service declares a healthcheck and none has a web port to probe")
	}
	if len(domains) > 0 {
		hc = &catalog.ManifestHealthcheck{Service: domains[0].Service, Port: domains[0].Port, Path: "/"}
	}

	composeBytes, err := marshal(outCompose{Services: c.out})
	if err != nil {
		return fail(ReasonValidationFailed, err.Error())
	}
	prompts := usedPrompts(c.t.prompts(), string(composeBytes))
	var images []catalog.ManifestImage
	for _, n := range c.names {
		images = append(images, catalog.ManifestImage{Service: n, Image: c.out[n].Image})
	}
	tpl := outTemplate{
		ID:                     id,
		Name:                   humanizeName(id),
		Description:            plainText(c.meta["slogan"]),
		Icon:                   iconFor(mapCategory(c.meta["category"])),
		Category:               mapCategory(c.meta["category"]),
		DocumentationURL:       strings.TrimSpace(c.meta["documentation"]),
		RecommendedMemoryBytes: estimateMemory(c.out),
		Source: &catalog.ManifestSource{
			Name: "coolify", URL: opt.SourceURL, License: opt.License, Path: opt.SourcePath,
		},
		Images: images, Env: toOutPrompts(prompts), Domains: domains, Healthcheck: hc,
	}
	tplBytes, err := marshal(tpl)
	if err != nil {
		return fail(ReasonValidationFailed, err.Error())
	}
	header := "# Converted from an upstream template catalog (see source in template.yaml).\n"
	composeBytes = append([]byte(header), composeBytes...)

	fsys := fstest.MapFS{
		"m/" + id + "/template.yaml": {Data: tplBytes},
		"m/" + id + "/compose.yaml":  {Data: composeBytes},
	}
	m, err := catalog.LoadManifest(fsys, "m/"+id)
	if err != nil {
		code, detail := classify(err)
		return fail(code, detail)
	}
	out.Imported = true
	out.Notes = c.t.notes
	out.Manifest = &m
	out.Template = tplBytes
	out.Compose = composeBytes
	out.Summary = c.summary(m, prompts, dports)
	return out
}

func (c *conversion) anyHealthcheck() bool {
	for _, o := range c.out {
		if o.Healthcheck != nil {
			return true
		}
	}
	return false
}

func (c *conversion) summary(m catalog.Manifest, prompts []catalog.EnvPrompt, dports map[string]int) *Summary {
	s := &Summary{Category: m.Category}
	s.Services = append(s.Services, c.names...)
	for _, n := range c.names {
		s.Images = append(s.Images, n+"="+c.out[n].Image)
		for _, p := range c.out[n].Ports {
			s.Ports = append(s.Ports, n+":"+p)
		}
	}
	for name, target := range c.named {
		s.Volumes = append(s.Volumes, name+":"+target)
	}
	sort.Strings(s.Volumes)
	for _, d := range m.Domains {
		s.Domains = append(s.Domains, fmt.Sprintf("%s:%d", d.Service, d.Port))
	}
	s.Generated = c.t.generatedSummary()
	for _, p := range prompts {
		if p.Generate == "" {
			s.UserPrompt = append(s.UserPrompt, p.Key)
		}
	}
	_ = dports
	return s
}

func toOutPrompts(in []catalog.EnvPrompt) []outPrompt {
	out := make([]outPrompt, len(in))
	for i, p := range in {
		out[i] = outPrompt(p)
	}
	return out
}

func usedPrompts(all []catalog.EnvPrompt, body string) []catalog.EnvPrompt {
	var out []catalog.EnvPrompt
	for _, p := range all {
		if strings.Contains(body, "${"+p.Key+"}") {
			out = append(out, p)
		}
	}
	return out
}

func classify(err error) (string, string) {
	msg := firstLine(flatten(err))
	switch {
	case strings.Contains(msg, "bind-mount host path"):
		return ReasonHostBindMount, msg
	case strings.Contains(msg, "is not allowed") || strings.Contains(msg, "weakens container isolation") || strings.Contains(msg, "host shares"):
		return ReasonHostAccess, msg
	}
	return ReasonValidationFailed, msg
}

func flatten(err error) string {
	var j interface{ Unwrap() []error }
	if errors.As(err, &j) {
		parts := make([]string, 0, len(j.Unwrap()))
		for _, e := range j.Unwrap() {
			parts = append(parts, flatten(e))
		}
		return strings.Join(parts, "; ")
	}
	return strings.ReplaceAll(strings.TrimSpace(err.Error()), "\n", " ")
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	if len(s) > 240 {
		s = s[:240]
	}
	return s
}

func parseHeader(src []byte) map[string]string {
	meta := map[string]string{}
	for _, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "#") {
			break
		}
		if m := headerPattern.FindStringSubmatch(line); m != nil {
			meta[m[1]] = strings.TrimSpace(m[2])
		}
	}
	return meta
}

func humanizeName(id string) string {
	parts := strings.Split(id, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}
