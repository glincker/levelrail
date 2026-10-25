// Package stacktemplates lists and renders the bundled starter stacks.
package stacktemplates

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/stacks"
)

// Template describes one bundle and the defaults of its variables.
type Template struct {
	Name        string
	Description string
	Image       string
}

var templates = []Template{
	{"agent-app", "App with a nightly scheduled job, a Postgres database and secret refs", "ghcr.io/acme/agent:latest"},
	{"preview-env", "Small app and database for one pull request, named per PR", "ghcr.io/acme/web:pr-1"},
	{"static-site-cdn", "nginx image app with a domain", "nginx:1.27-alpine"},
	{"web-app-postgres", "Web app, Postgres database and a domain", "ghcr.io/acme/web:latest"},
	{"worker-queue", "Web app, worker app and a Redis database", "ghcr.io/acme/app:latest"},
}

var placeholder = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_-]+)\s*\}\}`)

// List returns the templates sorted by name.
func List() []Template {
	out := append([]Template(nil), templates...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Find returns the template with the given name.
func Find(name string) (Template, bool) {
	for _, t := range templates {
		if t.Name == name {
			return t, true
		}
	}
	return Template{}, false
}

// Vars returns the variables for a template: defaults overlaid with set.
func (t Template) Vars(set map[string]string) (map[string]string, error) {
	vars := map[string]string{"name": t.Name, "image": t.Image}
	for k, v := range set {
		if k != "name" && k != "image" && k != "domain" {
			return nil, fmt.Errorf("unknown variable %q (known: name, domain, image)", k)
		}
		if strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("variable %q must not be empty", k)
		}
		vars[k] = v
	}
	if _, ok := vars["domain"]; !ok {
		vars["domain"] = vars["name"] + ".example.com"
	}
	return vars, nil
}

// Render returns the bundle files with variables substituted, keyed by file name.
func (t Template) Render(set map[string]string) (map[string][]byte, error) {
	vars, err := t.Vars(set)
	if err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(stacks.FS, t.Name)
	if err != nil {
		return nil, fmt.Errorf("read template %s: %w", t.Name, err)
	}
	out := make(map[string][]byte, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(stacks.FS, t.Name+"/"+e.Name())
		if err != nil {
			return nil, err
		}
		var missing string
		text := placeholder.ReplaceAllStringFunc(string(data), func(m string) string {
			key := placeholder.FindStringSubmatch(m)[1]
			v, ok := vars[key]
			if !ok {
				missing = key
				return m
			}
			return v
		})
		if missing != "" {
			return nil, fmt.Errorf("%s: unknown placeholder %q", e.Name(), missing)
		}
		out[e.Name()] = []byte(text)
	}
	return out, nil
}
