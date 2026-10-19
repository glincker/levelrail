// Package projecttpl renders starter project files (app.yaml and, for the
// pipeline starter, a pipeline file) from a small set of validated templates.
package projecttpl

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// Param kinds.
const (
	KindString = "string"
	KindInt    = "int"
	KindEnum   = "enum"
	KindCron   = "cron"
)

// Param is one value a template asks for.
type Param struct {
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Kind        string   `json:"kind"`
	Default     string   `json:"default,omitempty"`
	Required    bool     `json:"required"`
	Options     []string `json:"options,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
}

// Info describes a template without rendering it.
type Info struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Summary string  `json:"summary"`
	Params  []Param `json:"params"`
}

// File is one rendered file, path relative to the project root.
type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// FieldError ties a validation problem to the parameter that caused it.
type FieldError struct {
	Param   string `json:"param"`
	Message string `json:"message"`
}

// ValidationError lists every parameter problem at once.
type ValidationError struct {
	Fields []FieldError `json:"fields"`
}

func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		parts[i] = f.Param + ": " + f.Message
	}
	return "projecttpl: " + strings.Join(parts, "; ")
}

// ErrUnknownTemplate is returned for an id that is not in the catalog.
var ErrUnknownTemplate = errors.New("projecttpl: unknown template")

type template struct {
	Info
	render func(v map[string]string) []File
}

var nameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

var (
	nameParam   = Param{Name: "name", Label: "App name", Kind: KindString, Required: true, Placeholder: "my-app"}
	portParam   = Param{Name: "port", Label: "Port", Kind: KindInt, Default: "3000", Required: true}
	buildParam  = Param{Name: "build", Label: "Build method", Kind: KindEnum, Default: "railpack", Options: []string{"railpack", "dockerfile"}}
	healthParam = Param{Name: "health_path", Label: "Health check path", Kind: KindString, Default: "/"}
	idlePort    = Param{Name: "port", Label: "Port (the spec requires one, a worker need not listen)", Kind: KindInt, Default: "3000", Required: true}
	domainParam = Param{Name: "domain", Label: "Domain (optional)", Kind: KindString, Placeholder: "app.example.com"}
)

var catalog = []template{
	{Info: Info{ID: "web-service", Title: "Web service", Summary: "An HTTP app built from your repo with a health check.",
		Params: []Param{nameParam, portParam, buildParam, healthParam, domainParam}}, render: renderWeb},
	{Info: Info{ID: "static-site", Title: "Static site", Summary: "A static site served directly by the ingress, no container.",
		Params: []Param{nameParam, {Name: "path", Label: "Output directory", Kind: KindString, Default: "dist", Required: true}, domainParam}}, render: renderStatic},
	{Info: Info{ID: "worker", Title: "Background worker", Summary: "A long-running process with no port, restarted if it exits.",
		Params: []Param{nameParam, buildParam, idlePort, {Name: "command", Label: "Start command (optional)", Kind: KindString, Placeholder: "node worker.js"}}}, render: renderWorker},
	{Info: Info{ID: "cron-job", Title: "Scheduled job", Summary: "A command run on a cron schedule in a fresh container.",
		Params: []Param{nameParam, buildParam, idlePort, {Name: "schedule", Label: "Cron schedule", Kind: KindCron, Default: "0 3 * * *", Required: true},
			{Name: "command", Label: "Command", Kind: KindString, Required: true, Placeholder: "node jobs/nightly.js"}}}, render: renderCron},
	{Info: Info{ID: "build-test-deploy", Title: "Build, test and deploy", Summary: "A web service plus a pipeline that tests, builds and deploys on push.",
		Params: []Param{nameParam, portParam,
			{Name: "branch", Label: "Branch", Kind: KindString, Default: "main", Required: true},
			{Name: "test_image", Label: "Test image", Kind: KindString, Default: "node:22", Required: true},
			{Name: "test_command", Label: "Test command", Kind: KindString, Default: "npm test", Required: true}}}, render: renderPipeline},
}

// List returns the catalog in display order.
func List() []Info {
	out := make([]Info, len(catalog))
	for i, t := range catalog {
		out[i] = t.Info
	}
	return out
}

// Render validates values against the template's parameters, fills defaults
// and returns the files. The result is checked with the platform's own spec
// and pipeline parsers, so a template can never emit an invalid file.
func Render(id string, values map[string]string) ([]File, error) {
	for _, t := range catalog {
		if t.ID != id {
			continue
		}
		resolved, err := resolve(t.Params, values)
		if err != nil {
			return nil, err
		}
		files := t.render(resolved)
		if err := verify(files); err != nil {
			return nil, fmt.Errorf("projecttpl: template %s produced an invalid file: %w", id, err)
		}
		return files, nil
	}
	return nil, ErrUnknownTemplate
}

func resolve(params []Param, in map[string]string) (map[string]string, error) {
	out := map[string]string{}
	var bad []FieldError
	known := map[string]bool{}
	for _, p := range params {
		known[p.Name] = true
		v := strings.TrimSpace(in[p.Name])
		if v == "" {
			v = p.Default
		}
		if v == "" {
			if p.Required {
				bad = append(bad, FieldError{p.Name, "is required"})
			}
			continue
		}
		if msg := check(p, v); msg != "" {
			bad = append(bad, FieldError{p.Name, msg})
			continue
		}
		out[p.Name] = v
	}
	for k := range in {
		if !known[k] {
			bad = append(bad, FieldError{k, "is not a parameter of this template"})
		}
	}
	if len(bad) > 0 {
		sort.Slice(bad, func(i, j int) bool { return bad[i].Param < bad[j].Param })
		return nil, &ValidationError{Fields: bad}
	}
	return out, nil
}

func check(p Param, v string) string {
	if strings.ContainsAny(v, "\r\n") {
		return "must be a single line"
	}
	switch {
	case p.Name == "name":
		if !nameRe.MatchString(v) {
			return "use lowercase letters, digits and hyphens, starting and ending with a letter or digit"
		}
	case p.Kind == KindInt:
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			return "must be a port between 1 and 65535"
		}
	case p.Kind == KindEnum:
		for _, o := range p.Options {
			if o == v {
				return ""
			}
		}
		return "must be one of " + strings.Join(p.Options, ", ")
	case p.Kind == KindCron:
		if len(strings.Fields(v)) != 5 {
			return "must be a 5 field cron expression"
		}
	case p.Name == "health_path":
		if !strings.HasPrefix(v, "/") {
			return "must start with /"
		}
	case p.Name == "path":
		if strings.HasPrefix(v, "/") || strings.Contains(v, "..") {
			return "must be a relative path inside the repository"
		}
	}
	return ""
}

func verify(files []File) error {
	for _, f := range files {
		if f.Path == "app.yaml" {
			if _, err := spec.Parse([]byte(f.Content)); err != nil {
				return err
			}
			continue
		}
		if _, issues := pipeline.Validate([]byte(f.Content)); len(issues) > 0 {
			return errors.New(issues[0].String())
		}
	}
	return nil
}

// q renders s as a JSON string, which is also a valid YAML double-quoted
// scalar, so user values cannot break out of their line.
func q(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func buildBlock(kind string) string {
	return "    build:\n      type: " + kind + "\n"
}

func cmdList(s string) string {
	f := strings.Fields(s)
	parts := make([]string, len(f))
	for i, w := range f {
		parts[i] = q(w)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func domains(v map[string]string) string {
	if v["domain"] == "" {
		return ""
	}
	return "    domains:\n      - " + q(v["domain"]) + "\n"
}

func renderWeb(v map[string]string) []File {
	body := "version: 1\nservices:\n  " + v["name"] + ":\n" + buildBlock(v["build"]) +
		domains(v) + "    port: " + v["port"] + "\n    health:\n      readiness: { path: " + q(v["health_path"]) + ", interval: 5s, timeout: 2s }\n"
	return []File{{Path: "app.yaml", Content: body}}
}

func renderStatic(v map[string]string) []File {
	body := "version: 1\nservices:\n  " + v["name"] + ":\n    build:\n      type: static\n      path: " + q(v["path"]) + "\n" + domains(v)
	return []File{{Path: "app.yaml", Content: body}}
}

func renderWorker(v map[string]string) []File {
	body := "version: 1\nservices:\n  " + v["name"] + ":\n" + buildBlock(v["build"]) + "    port: " + v["port"] + "\n"
	if v["command"] != "" {
		body += "    command: " + cmdList(v["command"]) + "\n"
	}
	return []File{{Path: "app.yaml", Content: body}}
}

func renderCron(v map[string]string) []File {
	body := "version: 1\nservices:\n  " + v["name"] + ":\n" + buildBlock(v["build"]) + "    port: " + v["port"] + "\n" +
		"    jobs:\n      - name: " + q(v["name"]) + "\n        schedule: " + q(v["schedule"]) + "\n        command: " + cmdList(v["command"]) + "\n"
	return []File{{Path: "app.yaml", Content: body}}
}

func renderPipeline(v map[string]string) []File {
	app := "version: 1\nservices:\n  " + v["name"] + ":\n    build:\n      type: railpack\n    port: " + v["port"] +
		"\n    health:\n      readiness: { path: /, interval: 5s, timeout: 2s }\n"
	pipe := "version: 1\nname: release\non:\n  push:\n    branches: [" + q(v["branch"]) + "]\n  manual: {}\nstages: [test, build, deploy]\njobs:\n" +
		"  test:\n    stage: test\n    image: " + q(v["test_image"]) + "\n    steps:\n      - run: " + q(v["test_command"]) + "\n" +
		"  build:\n    stage: build\n    steps:\n      - uses: build\n        id: image\n" +
		"  ship:\n    stage: deploy\n    steps:\n      - uses: deploy\n        with:\n          strategy: blue-green\n"
	return []File{{Path: "app.yaml", Content: app}, {Path: ".pipelines/release.yaml", Content: pipe}}
}
