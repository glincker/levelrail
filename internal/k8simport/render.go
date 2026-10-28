package k8simport

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type outDoc struct {
	Version  int     `yaml:"version"`
	Kind     string  `yaml:"kind"`
	Metadata outMeta `yaml:"metadata"`
	Spec     any     `yaml:"spec"`
}

type outMeta struct {
	Name string `yaml:"name"`
}

type outApp struct {
	Project     string `yaml:"project,omitempty"`
	Environment string `yaml:"environment,omitempty"`
	Service     outSvc `yaml:"service"`
}

type outSvc struct {
	Build          outBuild          `yaml:"build"`
	Port           int               `yaml:"port,omitempty"`
	Replicas       int               `yaml:"replicas,omitempty"`
	Strategy       string            `yaml:"strategy,omitempty"`
	Command        []string          `yaml:"command,omitempty"`
	Restart        string            `yaml:"restart,omitempty"`
	Domains        []string          `yaml:"domains,omitempty"`
	PublishedPorts []outPub          `yaml:"published_ports,omitempty"`
	Resources      *outRes           `yaml:"resources,omitempty"`
	Health         *outHealth        `yaml:"health,omitempty"`
	Env            map[string]outEnv `yaml:"env,omitempty"`
	Volumes        []outVol          `yaml:"volumes,omitempty"`
	Jobs           []outJob          `yaml:"jobs,omitempty"`
}

type outBuild struct {
	Type  string `yaml:"type"`
	Image string `yaml:"image"`
}

type outPub struct {
	ContainerPort int    `yaml:"container_port"`
	HostPort      int    `yaml:"host_port"`
	Protocol      string `yaml:"protocol"`
}

type outRes struct {
	Memory string  `yaml:"memory,omitempty"`
	CPU    float64 `yaml:"cpu,omitempty"`
}

type outHealth struct {
	Readiness    *outProbe `yaml:"readiness,omitempty"`
	Liveness     *outProbe `yaml:"liveness,omitempty"`
	ReadyTimeout string    `yaml:"readyTimeout,omitempty"`
}

type outProbe struct {
	Path     string   `yaml:"path,omitempty"`
	Scheme   string   `yaml:"scheme,omitempty"`
	Exec     []string `yaml:"exec,omitempty"`
	Interval string   `yaml:"interval,omitempty"`
	Timeout  string   `yaml:"timeout,omitempty"`
	Failures int      `yaml:"failures,omitempty"`
}

type outVol struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

type outJob struct {
	Name        string   `yaml:"name"`
	Schedule    string   `yaml:"schedule"`
	Command     []string `yaml:"command,flow"`
	Timezone    string   `yaml:"timezone,omitempty"`
	Concurrency string   `yaml:"concurrency,omitempty"`
	Timeout     string   `yaml:"timeout,omitempty"`
	Enabled     *bool    `yaml:"enabled,omitempty"`
}

// outEnv is a literal or a secretRef, never a secret value.
type outEnv struct {
	literal string
	secret  string
}

var yaml11Bool = regexpYAML11()

func (v outEnv) MarshalYAML() (any, error) {
	if v.secret != "" {
		return &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "secretRef"}, {Kind: yaml.ScalarNode, Value: v.secret},
		}}, nil
	}
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v.literal}
	if yaml11Bool.MatchString(v.literal) {
		n.Style = yaml.DoubleQuotedStyle
	}
	return n, nil
}

func probeDoc(p *probeOut) *outProbe {
	if p == nil {
		return nil
	}
	return &outProbe{Path: p.path, Scheme: p.scheme, Exec: p.exec, Interval: p.interval, Timeout: p.timeout, Failures: p.failures}
}

func (im *importer) render() {
	sort.SliceStable(im.apps, func(i, j int) bool { return im.apps[i].name < im.apps[j].name })
	var docs []outDoc
	envs := map[string]bool{}
	for _, a := range im.apps {
		if a.port == 0 {
			for _, p := range a.containerPorts {
				if !strings.EqualFold(p.Protocol, "UDP") {
					a.port = p.ContainerPort
					break
				}
			}
		}
		if a.port == 0 && len(a.published) == 0 {
			a.port = DefaultPortFromEnv()
			im.res.add(SeverityChangesBehavior, a.kind, a.srcName, "ports", fmt.Sprintf("no port declared and every app needs one: %d is a placeholder, nothing has to listen on it", a.port))
		}
		svc := outSvc{Build: outBuild{Type: "image", Image: a.image}, Port: a.port, Replicas: a.replicas, Strategy: a.strategy,
			Command: a.command, Restart: a.restart, Domains: dedupe(a.domains)}
		sort.Slice(a.published, func(i, j int) bool { return a.published[i].host < a.published[j].host })
		for _, p := range a.published {
			svc.PublishedPorts = append(svc.PublishedPorts, outPub{ContainerPort: p.container, HostPort: p.host, Protocol: p.protocol})
		}
		if a.memory != "" || a.cpu > 0 {
			svc.Resources = &outRes{Memory: a.memory, CPU: a.cpu}
		}
		if a.readiness != nil || a.liveness != nil || a.readyTimeout != "" {
			svc.Health = &outHealth{Readiness: probeDoc(a.readiness), Liveness: probeDoc(a.liveness), ReadyTimeout: a.readyTimeout}
		}
		if len(a.env) > 0 {
			svc.Env = map[string]outEnv{}
			for k, v := range a.env {
				if v.secret {
					svc.Env[k] = outEnv{secret: k}
				} else {
					svc.Env[k] = outEnv{literal: v.literal}
				}
			}
		}
		seen := map[string]bool{}
		for _, v := range a.volumes {
			if !seen[v.path] {
				seen[v.path] = true
				svc.Volumes = append(svc.Volumes, outVol{Name: v.name, Path: v.path})
			}
		}
		for _, j := range a.jobs {
			oj := outJob{Name: j.name, Schedule: j.schedule, Command: j.command, Timezone: j.tz, Concurrency: j.concurrency, Timeout: j.timeout}
			if j.disabled {
				f := false
				oj.Enabled = &f
			}
			svc.Jobs = append(svc.Jobs, oj)
		}
		spec := outApp{Service: svc}
		if im.opts.NamespaceAs == NamespaceEnvironment {
			spec.Project, spec.Environment = im.project(), a.ns
			envs[a.ns] = true
		}
		docs = append(docs, outDoc{Version: 1, Kind: "App", Metadata: outMeta{Name: a.name}, Spec: spec})
		im.res.Apps = append(im.res.Apps, im.summary(a))
	}
	var head []outDoc
	if len(envs) > 0 {
		head = append(head, outDoc{Version: 1, Kind: "Project", Metadata: outMeta{Name: im.project()}, Spec: map[string]string{}})
		for _, e := range sortedMap(envs) {
			head = append(head, outDoc{Version: 1, Kind: "Environment", Metadata: outMeta{Name: e}, Spec: map[string]string{"project": im.project()}})
		}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	for _, d := range append(head, docs...) {
		_ = enc.Encode(d)
	}
	_ = enc.Close()
	im.res.Documents = buf.String()
}

func (im *importer) project() string {
	if im.opts.Project != "" {
		return im.opts.Project
	}
	return "kubernetes"
}

func (im *importer) summary(a *app) AppSummary {
	s := AppSummary{Name: a.name, Source: a.kind + "/" + a.srcName, Image: a.image, Port: a.port, Replicas: a.replicas, Domains: dedupe(a.domains)}
	for k, v := range a.env {
		if v.secret {
			s.SecretKeys = append(s.SecretKeys, k)
		} else {
			s.EnvKeys = append(s.EnvKeys, k)
		}
	}
	sort.Strings(s.EnvKeys)
	sort.Strings(s.SecretKeys)
	return s
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func regexpYAML11() *regexp.Regexp {
	return regexp.MustCompile(`(?i)^(y|n|yes|no|on|off|true|false|null|~)$`)
}
