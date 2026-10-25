package platformimport

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/envredact"
)

// Emitted file names, stable so a migration PR diffs cleanly.
const (
	EmitStackFile     = "stack.yaml"
	EmitProjectFile   = "project.yaml"
	EmitDatabasesFile = "databases.yaml"
	EmitAppsFile      = "apps.yaml"
	EmitDomainsFile   = "domains.yaml"
	EmitSecretsFile   = "SECRETS.md"
	EmitNotesFile     = "MIGRATION.md"
)

// EmitSecret is a secret the emitted documents reference by name. The value
// is never held here.
type EmitSecret struct {
	Name string
	Hint string
}

// EmitApp is one app to write as a kind: App document.
type EmitApp struct {
	Name string
	// Image is the prebuilt image. Empty means use EmitOptions.ImageRepo.
	Image       string
	Port        int
	Replicas    int
	Env         map[string]string
	Secrets     []EmitSecret
	Domains     []string
	MemoryBytes int64
	// Memory is a ready "512Mi" string and wins over MemoryBytes.
	Memory     string
	NanoCPUs   int64
	HealthPath string
	Command    []string
	PreDeploy  string
	PostDeploy string
	// Notes are follow-ups for MIGRATION.md that the documents cannot express.
	Notes []string
}

// EmitDatabase is one kind: Database document.
type EmitDatabase struct {
	Name    string
	Engine  string
	Version string
}

// EmitInput is everything an emit needs, from any source adapter.
type EmitInput struct {
	// Source names where the data came from, for the checklist header.
	Source    string
	Apps      []EmitApp
	Databases []EmitDatabase
	// Notes are source-wide follow-ups for MIGRATION.md.
	Notes []string
}

// EmitOptions name the stack and fill what the source does not know.
type EmitOptions struct {
	Stack       string
	Project     string
	Environment string
	// ImageRepo prefixes the image of an app that has none, as REPO/APP:latest.
	ImageRepo string
}

// EmitFile is one file of the emitted stack directory.
type EmitFile struct {
	Name    string
	Content []byte
}

type emitDoc struct {
	Version  int      `yaml:"version"`
	Kind     string   `yaml:"kind"`
	Metadata emitMeta `yaml:"metadata"`
	Spec     any      `yaml:"spec"`
}

type emitMeta struct {
	Name string `yaml:"name"`
}

type emitStackSpec struct {
	Project     string `yaml:"project"`
	Environment string `yaml:"environment,omitempty"`
}

type emitEnvSpec struct {
	Project string `yaml:"project"`
}

type emitDBSpec struct {
	Engine  string `yaml:"engine"`
	Version string `yaml:"version,omitempty"`
	Project string `yaml:"project,omitempty"`
}

type emitAppSpec struct {
	Project     string  `yaml:"project,omitempty"`
	Environment string  `yaml:"environment,omitempty"`
	Service     emitSvc `yaml:"service"`
}

type emitSvc struct {
	Build     emitBuild            `yaml:"build"`
	Port      int                  `yaml:"port,omitempty"`
	Replicas  int                  `yaml:"replicas,omitempty"`
	Command   []string             `yaml:"command,omitempty"`
	Resources *emitResources       `yaml:"resources,omitempty"`
	Health    *emitHealth          `yaml:"health,omitempty"`
	Hooks     *emitHooks           `yaml:"hooks,omitempty"`
	Env       map[string]emitValue `yaml:"env,omitempty"`
}

type emitBuild struct {
	Type  string `yaml:"type"`
	Image string `yaml:"image"`
}

type emitResources struct {
	Memory string  `yaml:"memory,omitempty"`
	CPU    float64 `yaml:"cpu,omitempty"`
}

type emitHealth struct {
	Readiness emitProbe `yaml:"readiness"`
}

type emitProbe struct {
	Path string `yaml:"path"`
}

type emitHooks struct {
	PreDeploy  string `yaml:"preDeploy,omitempty"`
	PostDeploy string `yaml:"postDeploy,omitempty"`
}

// emitValue is a literal env value or a secretRef by the same name.
type emitValue struct {
	literal   string
	secretRef string
}

func (v emitValue) MarshalYAML() (any, error) {
	if v.secretRef != "" {
		return &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "secretRef"}, {Kind: yaml.ScalarNode, Value: v.secretRef},
		}}, nil
	}
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v.literal}
	// YAML 1.1 readers turn yes, no, on and off into booleans, so quote them.
	if yaml11Bool.MatchString(v.literal) {
		n.Style = yaml.DoubleQuotedStyle
	}
	return n, nil
}

var yaml11Bool = regexp.MustCompile(`(?i)^(y|n|yes|no|on|off|true|false|null|~)$`)

type emitDomainSpec struct {
	App string `yaml:"app"`
}

// EmitStack renders the stack directory: Stack, Project, Database, App and
// Domain documents plus SECRETS.md (names only) and MIGRATION.md. Output is
// sorted so the same input always gives the same bytes.
func EmitStack(in EmitInput, o EmitOptions) ([]EmitFile, error) {
	if o.Stack == "" || o.Project == "" {
		return nil, errors.New("an emit needs a stack name and a project name")
	}
	apps := append([]EmitApp(nil), in.Apps...)
	sort.SliceStable(apps, func(i, j int) bool { return apps[i].Name < apps[j].Name })
	dbs := append([]EmitDatabase(nil), in.Databases...)
	sort.SliceStable(dbs, func(i, j int) bool { return dbs[i].Name < dbs[j].Name })

	var appDocs, domainDocs, dbDocs []emitDoc
	notes := append([]string(nil), in.Notes...)
	owner := map[string]string{}
	for i := range apps {
		a := &apps[i]
		doc, secrets, err := emitAppDoc(a, o)
		if err != nil {
			return nil, err
		}
		a.Secrets = secrets
		appDocs = append(appDocs, doc)
		for _, h := range sortedUniqueStrings(a.Domains) {
			if prev, dup := owner[h]; dup {
				notes = append(notes, fmt.Sprintf("domain %s is claimed by %s and %s, it is emitted for %s only", h, prev, a.Name, prev))
				continue
			}
			owner[h] = a.Name
			domainDocs = append(domainDocs, emitDoc{Version: 1, Kind: "Domain", Metadata: emitMeta{Name: h}, Spec: emitDomainSpec{App: a.Name}})
		}
	}
	for _, d := range dbs {
		dbDocs = append(dbDocs, emitDoc{Version: 1, Kind: "Database", Metadata: emitMeta{Name: d.Name}, Spec: emitDBSpec{Engine: d.Engine, Version: d.Version, Project: o.Project}})
	}

	head := []emitDoc{{Version: 1, Kind: "Project", Metadata: emitMeta{Name: o.Project}, Spec: map[string]string{}}}
	if o.Environment != "" {
		head = append(head, emitDoc{Version: 1, Kind: "Environment", Metadata: emitMeta{Name: o.Environment}, Spec: emitEnvSpec{Project: o.Project}})
	}
	var files []EmitFile
	for _, g := range []struct {
		name string
		docs []emitDoc
	}{
		{EmitStackFile, []emitDoc{{Version: 1, Kind: "Stack", Metadata: emitMeta{Name: o.Stack}, Spec: emitStackSpec{Project: o.Project, Environment: o.Environment}}}},
		{EmitProjectFile, head}, {EmitDatabasesFile, dbDocs}, {EmitAppsFile, appDocs}, {EmitDomainsFile, domainDocs},
	} {
		if len(g.docs) == 0 {
			continue
		}
		b, err := marshalDocs(g.docs)
		if err != nil {
			return nil, err
		}
		files = append(files, EmitFile{Name: g.name, Content: b})
	}
	files = append(files,
		EmitFile{Name: EmitSecretsFile, Content: []byte(renderSecrets(in.Source, o, apps, dbs))},
		EmitFile{Name: EmitNotesFile, Content: []byte(renderNotes(in.Source, o, apps, notes))})
	return files, nil
}

func emitAppDoc(a *EmitApp, o EmitOptions) (emitDoc, []EmitSecret, error) {
	image := a.Image
	if image == "" {
		if o.ImageRepo == "" {
			return emitDoc{}, nil, fmt.Errorf("app %s has no image: pass --image-repo REGISTRY/NAMESPACE so its image is named REGISTRY/NAMESPACE/%s:latest", a.Name, a.Name)
		}
		image = strings.TrimRight(o.ImageRepo, "/") + "/" + a.Name + ":latest"
	}
	if a.Port == 0 {
		a.Port = 80
		a.Notes = append(a.Notes, "the source names no port, 80 is assumed: set the port the image listens on")
	}
	svc := emitSvc{Build: emitBuild{Type: "image", Image: image}, Port: a.Port, Replicas: a.Replicas, Command: a.Command}
	if a.MemoryBytes > 0 || a.NanoCPUs > 0 || a.Memory != "" {
		r := &emitResources{CPU: float64(a.NanoCPUs) / 1e9, Memory: a.Memory}
		if a.MemoryBytes > 0 && a.Memory == "" {
			r.Memory = fmt.Sprintf("%dMi", (a.MemoryBytes+(1<<20)-1)>>20)
		}
		svc.Resources = r
	}
	if a.HealthPath != "" {
		svc.Health = &emitHealth{Readiness: emitProbe{Path: a.HealthPath}}
	}
	if a.PreDeploy != "" || a.PostDeploy != "" {
		svc.Hooks = &emitHooks{PreDeploy: a.PreDeploy, PostDeploy: a.PostDeploy}
	}
	env := map[string]emitValue{}
	secrets := map[string]string{}
	for _, s := range a.Secrets {
		secrets[s.Name] = s.Hint
	}
	for k, v := range a.Env {
		if envredact.SensitiveName(k) || envredact.SensitiveValue(k, v) {
			if _, ok := secrets[k]; !ok {
				secrets[k] = "looked like a secret on the source, so it is a reference here"
			}
			continue
		}
		env[k] = emitValue{literal: v}
	}
	names := make([]string, 0, len(secrets))
	for k := range secrets {
		names = append(names, k)
		env[k] = emitValue{secretRef: k}
	}
	sort.Strings(names)
	out := make([]EmitSecret, 0, len(names))
	for _, k := range names {
		out = append(out, EmitSecret{Name: k, Hint: secrets[k]})
	}
	if len(env) > 0 {
		svc.Env = env
	}
	spec := emitAppSpec{Project: o.Project, Environment: o.Environment, Service: svc}
	return emitDoc{Version: 1, Kind: "App", Metadata: emitMeta{Name: a.Name}, Spec: spec}, out, nil
}

func marshalDocs(docs []emitDoc) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	for _, d := range docs {
		if err := enc.Encode(d); err != nil {
			return nil, fmt.Errorf("platformimport: encode %s %s: %w", d.Kind, d.Metadata.Name, err)
		}
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("platformimport: encode documents: %w", err)
	}
	return buf.Bytes(), nil
}

func sortedUniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func renderSecrets(source string, o EmitOptions, apps []EmitApp, dbs []EmitDatabase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Secrets checklist for stack %s\n\n", o.Stack)
	fmt.Fprintf(&b, "Source: %s. This file lists secret names only. No value was read into these files, and none belongs in this repository.\n\n", source)
	b.WriteString("Supply each value at apply time from your CI secret store, for example:\n\n")
	example := "NAME"
	for _, a := range apps {
		if len(a.Secrets) > 0 {
			example = a.Name + "/" + a.Secrets[0].Name
			break
		}
	}
	fmt.Fprintf(&b, "```bash\nlevelrail-cli stack apply %s -f . --yes --secret %s=env:MY_CI_SECRET\n```\n\n", o.Stack, example)
	b.WriteString("Use `NAME` to set a value for every app that references it, or `app/NAME` for one app.\n")
	total := 0
	for _, a := range apps {
		if len(a.Secrets) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## App %s\n\n", a.Name)
		for _, s := range a.Secrets {
			total++
			fmt.Fprintf(&b, "- [ ] `%s`", s.Name)
			if s.Hint != "" {
				fmt.Fprintf(&b, ": %s", s.Hint)
			}
			b.WriteString("\n")
		}
	}
	if total == 0 {
		b.WriteString("\nNo secret names were found.\n")
	}
	if len(dbs) > 0 {
		b.WriteString("\n## Database connections\n\n")
		b.WriteString("The documents cannot yet hand a database's connection string to an app (`databases:` sections and `from: postgres.main.url` references arrive with a later schema change). Until then each connection variable above is a secret name placeholder. Create the databases from `databases.yaml`, read each connection string, and supply it as the secret named above, or run `levelrail-cli databases link NAME APP` so the platform injects it.\n\n")
		for _, d := range dbs {
			fmt.Fprintf(&b, "- `%s` (%s)\n", d.Name, d.Engine)
		}
	}
	return b.String()
}

func renderNotes(source string, o EmitOptions, apps []EmitApp, general []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Migration notes for stack %s\n\n", o.Stack)
	fmt.Fprintf(&b, "Generated from %s. Review the documents in a pull request, then run `levelrail-cli stack plan %s -f .` before the first apply.\n", source, o.Stack)
	b.WriteString("\n## What the files do not do\n\n")
	b.WriteString("- Apply runs prebuilt images only. An app with no image on the source is named after the `--image-repo` you gave, and nothing builds it: push that image from your CI first.\n")
	b.WriteString("- Secret values, volumes, scheduled jobs and database contents are not carried over by the documents.\n")
	for _, n := range general {
		fmt.Fprintf(&b, "- %s\n", n)
	}
	for _, a := range apps {
		if len(a.Notes) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## App %s\n\n", a.Name)
		for _, n := range a.Notes {
			fmt.Fprintf(&b, "- %s\n", n)
		}
	}
	return b.String()
}
