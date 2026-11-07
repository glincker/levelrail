package configtranslate

import (
	"encoding/json"
	"sort"
	"strings"
)

// VercelSummary is the plain-language shape of a project, for the guided import.
type VercelSummary struct {
	Framework string   `json:"framework,omitempty"`
	Build     string   `json:"build_command,omitempty"`
	Output    string   `json:"output_directory,omitempty"`
	Rewrites  int      `json:"rewrites"`
	Redirects int      `json:"redirects"`
	Headers   int      `json:"headers"`
	Crons     int      `json:"crons"`
	Regions   []string `json:"regions_ignored,omitempty"`
	// EnvKeys are build.env names from vercel.json (values are never kept).
	EnvKeys []string `json:"env_keys,omitempty"`
}

// SummarizeVercel reads vercel.json and package.json from files; a missing or invalid file is skipped.
func SummarizeVercel(files map[string][]byte) VercelSummary {
	var s VercelSummary
	if raw, ok := lookup(files, fileVercel); ok {
		var v vercelJSON
		if json.Unmarshal(raw, &v) == nil {
			s.Framework, s.Build, s.Output = str(v.Framework), str(v.BuildCommand), str(v.OutputDirectory)
			s.Rewrites, s.Redirects, s.Headers, s.Crons = len(v.Rewrites), len(v.Redirects), len(v.Headers), len(v.Crons)
			s.Regions = v.Regions
			for k := range v.Build.Env {
				s.EnvKeys = append(s.EnvKeys, k)
			}
			sort.Strings(s.EnvKeys)
		}
	}
	if raw, ok := lookup(files, "package.json"); ok {
		var p struct {
			Scripts      map[string]string `json:"scripts"`
			Dependencies map[string]string `json:"dependencies"`
			DevDeps      map[string]string `json:"devDependencies"`
		}
		if json.Unmarshal(raw, &p) == nil {
			if s.Framework == "" {
				s.Framework = detectJSFramework(p.Dependencies, p.DevDeps)
			}
			if s.Build == "" && p.Scripts["build"] != "" {
				s.Build = p.Scripts["build"]
			}
		}
	}
	return s
}

func detectJSFramework(deps ...map[string]string) string {
	for _, name := range []string{"next", "astro", "vite", "nuxt", "@sveltejs/kit", "gatsby"} {
		for _, d := range deps {
			if _, ok := d[name]; ok {
				return strings.TrimPrefix(name, "@sveltejs/")
			}
		}
	}
	return ""
}

// IsBuildTimeEnvKey reports keys that frameworks inline into the bundle, so they must be present at build.
func IsBuildTimeEnvKey(k string) bool {
	return strings.HasPrefix(k, "NEXT_PUBLIC_") || strings.HasPrefix(k, "VITE_")
}

// RoutingRules returns the translated redirects, rewrites and header rules across all services.
func (r Result) RoutingRules() ([]Route, []HeaderRule) {
	var routes []Route
	var headers []HeaderRule
	for _, n := range sortedKeys(r.Fragment.Services) {
		routes = append(routes, r.Fragment.Services[n].Routes...)
		headers = append(headers, r.Fragment.Services[n].Headers...)
	}
	return routes, headers
}

// Untranslated returns the findings an operator must handle by hand.
func (r Result) Untranslated() []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Severity == SeverityWarn || f.Severity == SeverityBlocker {
			out = append(out, f)
		}
	}
	return out
}

// VercelCutoverChecklist exposes the ordered cutover notes.
func VercelCutoverChecklist() []string { return vercelCutoverChecklist() }
