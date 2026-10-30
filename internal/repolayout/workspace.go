package repolayout

import (
	"encoding/json"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/kit/pathfilter"
)

// EnvMaxPackages caps how many workspace packages one analysis returns.
const EnvMaxPackages = "APP_WIZARD_MAX_PACKAGES"

const defaultMaxPackages = 60

// PackageCap is the effective package cap, read from the environment on each call.
func PackageCap() int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(EnvMaxPackages))); err == nil && v > 0 {
		return v
	}
	return defaultMaxPackages
}

// Package is one workspace member that can run as its own service.
type Package struct {
	Name       string   `json:"name"`
	Dir        string   `json:"dir"`
	Tool       string   `json:"tool"`
	Dockerfile string   `json:"dockerfile,omitempty"`
	DependsOn  []string `json:"depends_on"`
	// WatchPaths is the deploy filter that redeploys only this package:
	// its own directory, its workspace dependencies and the shared root files.
	WatchPaths []string `json:"watch_paths"`
}

// sharedRootFiles change every package's install or build graph.
var sharedRootFiles = []string{
	"package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", "package-lock.json", "yarn.lock", "bun.lockb",
	"turbo.json", "nx.json", "lerna.json", "tsconfig.base.json", "go.work", "go.work.sum", "go.mod", "go.sum",
}

var packageManifests = []string{"package.json", "go.mod", "project.json"}

// SharedPaths lists the root files present in the repository that affect all packages.
func sharedPaths(present map[string]bool) []string {
	out := []string{}
	for _, f := range sharedRootFiles {
		if present[f] {
			out = append(out, f)
		}
	}
	return out
}

// workspaceGlobs reads member globs from the workspace files that are present.
func workspaceGlobs(present map[string]bool, read Reader) (globs []string, tool string) {
	if read == nil {
		return nil, ""
	}
	if present["pnpm-workspace.yaml"] {
		if b, err := read("pnpm-workspace.yaml", 16*1024); err == nil {
			globs = append(globs, parsePnpmPackages(string(b))...)
			tool = "pnpm-workspace"
		}
	}
	if present["package.json"] {
		if b, err := read("package.json", 64*1024); err == nil {
			if g := parseNpmWorkspaces(b); len(g) > 0 {
				globs = append(globs, g...)
				if tool == "" {
					tool = "npm-workspaces"
				}
			}
		}
	}
	if present["go.work"] {
		if b, err := read("go.work", 16*1024); err == nil {
			if g := parseGoWork(string(b)); len(g) > 0 {
				globs = append(globs, g...)
				if tool == "" {
					tool = "go-work"
				}
			}
		}
	}
	return globs, tool
}

func parsePnpmPackages(text string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(trim, "- ") {
			in = strings.HasPrefix(trim, "packages:")
			continue
		}
		if in && strings.HasPrefix(trim, "- ") {
			v := strings.Trim(strings.TrimSpace(strings.TrimPrefix(trim, "- ")), `'"`)
			if v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

func parseNpmWorkspaces(b []byte) []string {
	var doc struct {
		Workspaces json.RawMessage `json:"workspaces"`
	}
	if json.Unmarshal(b, &doc) != nil || len(doc.Workspaces) == 0 {
		return nil
	}
	var list []string
	if json.Unmarshal(doc.Workspaces, &list) == nil {
		return list
	}
	var obj struct {
		Packages []string `json:"packages"`
	}
	if json.Unmarshal(doc.Workspaces, &obj) == nil {
		return obj.Packages
	}
	return nil
}

func parseGoWork(text string) []string {
	var out []string
	block := false
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case block && t == ")":
			block = false
		case block:
			out = append(out, cleanGoUse(t))
		case t == "use (":
			block = true
		case strings.HasPrefix(t, "use "):
			out = append(out, cleanGoUse(strings.TrimPrefix(t, "use ")))
		}
	}
	return out
}

func cleanGoUse(s string) string {
	s = strings.TrimSpace(strings.SplitN(s, "//", 2)[0])
	return strings.TrimPrefix(strings.Trim(s, `"`), "./")
}

// findPackages expands workspace globs against the file list. A directory is a
// package when it holds a manifest and matches a positive glob.
func findPackages(files []string, present map[string]bool, read Reader) []Package {
	pkgs, _ := findPackagesTotal(files, present, read)
	return pkgs
}

// findPackagesTotal also returns how many packages matched before the cap.
func findPackagesTotal(files []string, present map[string]bool, read Reader) ([]Package, int) {
	globs, tool := workspaceGlobs(present, read)
	if len(globs) == 0 {
		return []Package{}, 0
	}
	dirs := map[string]bool{}
	for _, f := range files {
		if !contains(packageManifests, path.Base(f)) || path.Dir(f) == "." {
			continue
		}
		dirs[path.Dir(f)] = true
	}
	var matched []string
	for d := range dirs {
		if inWorkspace(globs, d) {
			matched = append(matched, d)
		}
	}
	sort.Strings(matched)
	total := len(matched)
	if limit := PackageCap(); total > limit {
		matched = matched[:limit]
	}

	names := map[string]string{}
	manifests := map[string][]byte{}
	for _, d := range matched {
		if present[d+"/package.json"] && read != nil {
			if b, err := read(d+"/package.json", 64*1024); err == nil {
				manifests[d] = b
				var pj struct {
					Name string `json:"name"`
				}
				if json.Unmarshal(b, &pj) == nil && pj.Name != "" {
					names[pj.Name] = d
				}
			}
		}
	}

	shared := sharedPaths(present)
	pkgs := make([]Package, 0, len(matched))
	for _, d := range matched {
		p := Package{Name: path.Base(d), Dir: d, Tool: tool, DependsOn: workspaceDeps(manifests[d], names, d)}
		if present[d+"/Dockerfile"] {
			p.Dockerfile = d + "/Dockerfile"
		}
		p.WatchPaths = append([]string{d + "/**"}, depGlobs(p.DependsOn)...)
		p.WatchPaths = append(p.WatchPaths, shared...)
		pkgs = append(pkgs, p)
	}
	return pkgs, total
}

// inWorkspace applies positive globs, then "!" exclusions, in order.
func inWorkspace(globs []string, dir string) bool {
	in := false
	for _, g := range globs {
		neg := strings.HasPrefix(g, "!")
		g = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(g, "!"), "./"), "/")
		if pathfilter.Match(g, dir) {
			in = !neg
		}
	}
	return in
}

func workspaceDeps(manifest []byte, names map[string]string, self string) []string {
	out := []string{}
	if len(manifest) == 0 {
		return out
	}
	var pj struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if json.Unmarshal(manifest, &pj) != nil {
		return out
	}
	seen := map[string]bool{}
	for _, m := range []map[string]string{pj.Dependencies, pj.DevDependencies} {
		for n := range m {
			if dir, ok := names[n]; ok && dir != self && !seen[dir] {
				seen[dir] = true
				out = append(out, dir)
			}
		}
	}
	sort.Strings(out)
	return out
}

func depGlobs(dirs []string) []string {
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		out = append(out, d+"/**")
	}
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
