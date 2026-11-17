// Command validate-registry checks registry/templates/<id> folders against the
// contribution rules in docs/registry-contributing.md.
//
// Usage (from the repo root): go run ./scripts/validate-registry
// Without -changed it only prints a legacy summary and exits 0, so folders that
// predate the rules never block CI. With -changed <file> (one path per line, "-"
// for stdin) only the touched template folders are enforced.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
)

// Problem is one rule violation in one template folder.
type Problem struct {
	Template string `json:"template"`
	Rule     string `json:"rule"`
	Message  string `json:"message"`
}

// Rule names, stable so CI output and the legacy summary can group on them.
const (
	ruleID       = "id"
	ruleFields   = "required-field"
	ruleCategory = "category"
	ruleCompose  = "compose"
	ruleImage    = "image-pin"
	ruleHealth   = "healthcheck"
	ruleSecurity = "security"
	rulePorts    = "ports"
	ruleIcon     = "icon"
)

var (
	idRe     = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	iconRe   = regexp.MustCompile(`^[a-z0-9-]+$`)
	interpRe = regexp.MustCompile(`\$\{[^}]*\}`)
)

// meta is read leniently so the validator does not depend on the loader schema.
type meta struct {
	ID               string `yaml:"id"`
	Name             string `yaml:"name"`
	Slogan           string `yaml:"slogan"`
	Category         string `yaml:"category"`
	DocumentationURL string `yaml:"documentation_url"`
	Icon             string `yaml:"icon"`
}

// knownCategories are the categories the shipped catalog already uses.
func knownCategories() map[string]bool {
	out := map[string]bool{}
	for _, t := range catalog.Templates {
		out[t.Category] = true
	}
	return out
}

func main() {
	root := flag.String("registry", "registry/templates", "registry templates directory")
	changed := flag.String("changed", "", "file listing changed paths, one per line (- for stdin); enforce only those template folders")
	asJSON := flag.Bool("json", false, "print problems as JSON")
	strict := flag.Bool("strict", false, "enforce every folder, not only changed ones")
	flag.Parse()
	os.Exit(run(*root, *changed, *asJSON, *strict, os.Stdout, os.Stderr))
}

func run(root, changed string, asJSON, strict bool, out, errOut io.Writer) int {
	ids, err := listIDs(root)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 2
	}
	enforce := ids
	if changed != "" {
		enforce, err = changedIDs(changed, root, ids)
		if err != nil {
			_, _ = fmt.Fprintln(errOut, err)
			return 2
		}
	} else if !strict {
		enforce = nil
	}
	cats := knownCategories()
	if len(enforce) > 0 {
		problems := validateAll(root, enforce, cats)
		emit(out, root, problems, asJSON)
		if len(problems) > 0 {
			return 1
		}
		return 0
	}
	legacy := validateAll(root, ids, cats)
	if asJSON {
		emit(out, root, legacy, true)
	} else {
		summary(out, len(ids), legacy)
	}
	return 0
}

func validateAll(root string, ids []string, cats map[string]bool) []Problem {
	var all []Problem
	for _, id := range ids {
		all = append(all, validateFolder(filepath.Join(root, id), id, cats)...)
	}
	return all
}

func emit(out io.Writer, root string, problems []Problem, asJSON bool) {
	if asJSON {
		if problems == nil {
			problems = []Problem{}
		}
		b, _ := json.MarshalIndent(problems, "", "  ")
		_, _ = fmt.Fprintln(out, string(b))
		return
	}
	prefix := filepath.ToSlash(root)
	for _, p := range problems {
		_, _ = fmt.Fprintf(out, "%s/%s: %s\n", prefix, p.Template, p.Message)
	}
}

func summary(out io.Writer, total int, problems []Problem) {
	failing := map[string]bool{}
	byRule := map[string]map[string]bool{}
	for _, p := range problems {
		failing[p.Template] = true
		if byRule[p.Rule] == nil {
			byRule[p.Rule] = map[string]bool{}
		}
		byRule[p.Rule][p.Template] = true
	}
	_, _ = fmt.Fprintf(out, "legacy summary: %d of %d templates break at least one rule (not enforced; only changed folders are)\n", len(failing), total)
	rules := make([]string, 0, len(byRule))
	for r := range byRule {
		rules = append(rules, r)
	}
	sort.Strings(rules)
	for _, r := range rules {
		_, _ = fmt.Fprintf(out, "  %s: %d templates\n", r, len(byRule[r]))
	}
}

func listIDs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read registry %s: %w", root, err)
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// changedIDs maps changed file paths to existing template folder names.
// A deleted folder is skipped since there is nothing left to validate.
func changedIDs(list, root string, existing []string) ([]string, error) {
	var r io.Reader = os.Stdin
	if list != "-" {
		f, err := os.Open(list) //nolint:gosec // path chosen by the CI caller
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	have := map[string]bool{}
	for _, id := range existing {
		have[id] = true
	}
	marker := filepath.ToSlash(root) + "/"
	seen := map[string]bool{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		p := filepath.ToSlash(strings.TrimSpace(sc.Text()))
		i := strings.Index(p, marker)
		if i < 0 {
			continue
		}
		id, _, _ := strings.Cut(p[i+len(marker):], "/")
		if have[id] {
			seen[id] = true
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func validateFolder(folder, id string, cats map[string]bool) []Problem {
	var ps []Problem
	add := func(rule, format string, args ...any) {
		ps = append(ps, Problem{Template: id, Rule: rule, Message: fmt.Sprintf(format, args...)})
	}
	if !idRe.MatchString(id) {
		add(ruleID, "folder name %q must be lowercase kebab case", id)
	}
	raw, err := os.ReadFile(filepath.Join(folder, catalog.RegistryMetaFile)) //nolint:gosec // registry folder chosen by the caller
	if err != nil {
		add(ruleFields, "missing %s", catalog.RegistryMetaFile)
		return ps
	}
	var m meta
	if err := yaml.Unmarshal(raw, &m); err != nil {
		add(ruleFields, "%s does not parse: %v", catalog.RegistryMetaFile, err)
		return ps
	}
	if m.ID != id {
		add(ruleID, "id %q must equal the folder name %q", m.ID, id)
	}
	for field, v := range map[string]string{"name": m.Name, "slogan": m.Slogan, "category": m.Category, "documentation_url": m.DocumentationURL} {
		if strings.TrimSpace(v) == "" {
			add(ruleFields, "missing required field %s", field)
		}
	}
	if m.Category != "" && !cats[m.Category] {
		add(ruleCategory, "unknown category %q", m.Category)
	}
	if m.Icon != "" && !iconRe.MatchString(m.Icon) {
		add(ruleIcon, "icon %q must match ^[a-z0-9-]+$", m.Icon)
	}
	body, err := os.ReadFile(filepath.Join(folder, catalog.RegistryComposeFile)) //nolint:gosec // registry folder chosen by the caller
	if err != nil {
		add(ruleFields, "missing required field compose (%s)", catalog.RegistryComposeFile)
		return ps
	}
	return append(ps, validateCompose(id, body)...)
}

func validateCompose(id string, body []byte) []Problem {
	var ps []Problem
	add := func(rule, format string, args ...any) {
		ps = append(ps, Problem{Template: id, Rule: rule, Message: fmt.Sprintf(format, args...)})
	}
	f, err := compose.Parse(body)
	if err == nil {
		err = f.Validate()
	}
	if err != nil {
		add(ruleCompose, "compose is not valid: %v", err)
	}
	var doc struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	if yaml.Unmarshal(body, &doc) != nil {
		return ps
	}
	names := make([]string, 0, len(doc.Services))
	for n := range doc.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	completesByDesign := completedDeps(doc.Services)
	for _, n := range names {
		svc := doc.Services[n]
		if img, ok := svc["image"].(string); ok {
			if msg := unpinned(img); msg != "" {
				add(ruleImage, "service %s: image %q %s", n, img, msg)
			}
		}
		if !hasHealthcheck(svc) && !completesByDesign[n] {
			add(ruleHealth, "service %s: no healthcheck", n)
		}
		if p, _ := svc["privileged"].(bool); p {
			add(ruleSecurity, "service %s: privileged is not allowed", n)
		}
		if nm, _ := svc["network_mode"].(string); nm == "host" {
			add(ruleSecurity, "service %s: host network is not allowed", n)
		}
		for _, src := range bindSources(svc["volumes"]) {
			if isHostRoot(src) || isDockerSocket(src) {
				add(ruleSecurity, "service %s: mounting %s is never allowed", n, src)
			}
		}
		if pub, ok := svc["ports"].([]any); ok {
			for _, p := range pub {
				if fixedHostPort(p) {
					add(rulePorts, "service %s: published port %v must be container-only, for example \"3000\"", n, p)
				}
			}
		}
	}
	return ps
}

// completedDeps names the one-shot jobs other services wait on until they exit
// successfully; such a job cannot report healthy, so it needs no healthcheck.
func completedDeps(services map[string]map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, svc := range services {
		deps, _ := svc["depends_on"].(map[string]any)
		for name, v := range deps {
			if c, _ := v.(map[string]any); c["condition"] == "service_completed_successfully" {
				out[name] = true
			}
		}
	}
	return out
}

func hasHealthcheck(svc map[string]any) bool {
	hc, ok := svc["healthcheck"].(map[string]any)
	if !ok {
		return false
	}
	if d, _ := hc["disable"].(bool); d {
		return false
	}
	return hc["test"] != nil
}

// unpinned returns why an image reference is not pinned, or "" when it is.
func unpinned(img string) string {
	if strings.Contains(img, "@sha256:") {
		return ""
	}
	// Only the last path element can carry a tag; earlier colons are registry ports.
	last := img[strings.LastIndex(img, "/")+1:]
	_, tag, ok := strings.Cut(last, ":")
	if !ok {
		return "has no tag or digest"
	}
	if strings.HasPrefix(tag, "${") {
		if _, def, has := strings.Cut(tag, ":-"); has {
			tag = strings.TrimSuffix(def, "}")
		} else {
			return ""
		}
	}
	if tag == "" || strings.EqualFold(tag, "latest") {
		return "uses the latest tag, pin a version or digest"
	}
	return ""
}

func bindSources(v any) []string {
	list, _ := v.([]any)
	var out []string
	for _, e := range list {
		switch x := e.(type) {
		case string:
			if src, _, ok := strings.Cut(x, ":"); ok {
				out = append(out, src)
			}
		case map[string]any:
			if t, _ := x["type"].(string); t == "bind" {
				if s, _ := x["source"].(string); s != "" {
					out = append(out, s)
				}
			}
		}
	}
	return out
}

func isHostRoot(p string) bool { return p == "/" }

func isDockerSocket(p string) bool {
	return strings.HasSuffix(p, "/docker.sock")
}

// fixedHostPort reports a ports: entry that pins a host port. Interpolations
// are masked first so ${PORT:-80}:80 counts its real colon only.
func fixedHostPort(p any) bool {
	switch x := p.(type) {
	case string:
		s := interpRe.ReplaceAllString(x, "X")
		s, _, _ = strings.Cut(s, "/")
		return strings.Contains(s, ":")
	case map[string]any:
		return x["published"] != nil
	}
	return false
}
