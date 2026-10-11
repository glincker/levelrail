// Package repolayout inspects a repository's file tree to suggest where its
// build context and Dockerfile should point, so a monorepo does not silently
// fall back to auto-detecting the repository root.
package repolayout

import (
	"path"
	"sort"
	"strings"
)

// Suggestion reason codes. The dashboard translates these; Reason carries the
// same sentence in English for the CLI and API consumers.
const (
	ReasonTurboPrune     = "turbo_prune"
	ReasonRootCopy       = "root_copy"
	ReasonAppDeployDir   = "app_deploy_dir"
	ReasonRootDockerfile = "root_dockerfile"
	ReasonNestedDocker   = "nested_dockerfile"
	ReasonAppRailpack    = "app_railpack"
)

const (
	buildTypeDockerfile = "dockerfile"
	buildTypeRailpack   = "railpack"

	maxSuggestions = 12
	maxAppDirs     = 6
	maxDockerfiles = 40
)

// Suggestion is one candidate build setting.
type Suggestion struct {
	BuildType      string `json:"build_type"`
	DockerfilePath string `json:"dockerfile_path,omitempty"`
	BaseDirectory  string `json:"base_directory,omitempty"`
	Reason         string `json:"reason"`
	ReasonCode     string `json:"reason_code"`
	Recommended    bool   `json:"recommended"`
	Score          int    `json:"-"`
}

// Result is what detection found in one repository at one ref.
type Result struct {
	LooksLikeMonorepo bool         `json:"looks_like_monorepo"`
	RootHasApp        bool         `json:"root_has_app"`
	Tools             []string     `json:"tools"`
	Dockerfiles       []string     `json:"dockerfiles"`
	ComposeFiles      []string     `json:"compose_files"`
	Suggestions       []Suggestion `json:"suggestions"`
	Truncated         bool         `json:"truncated"`
}

// Reader returns the first max bytes of a file, or an error when it cannot be
// read. Analyze treats a read failure as "no signal", never as a failure.
type Reader func(p string, limit int) ([]byte, error)

var ignoredDirs = map[string]bool{
	"node_modules": true, "vendor": true, ".git": true, ".next": true, ".turbo": true,
	"dist": true, "target": true, "bower_components": true, ".venv": true, "__pycache__": true,
	"testdata": true, "fixtures": true,
}

var appParentDirs = map[string]bool{"apps": true, "packages": true, "services": true, "cmd": true, "projects": true}

var deployDirs = map[string]bool{"deploy": true, "docker": true, ".docker": true, "build": true, "deployment": true, "infra": true}

var rootAppFiles = []string{"go.mod", "requirements.txt", "pyproject.toml", "pom.xml", "build.gradle", "composer.json", "Gemfile", "mix.exs", "index.html"}

// Analyze inspects the file paths of a repository (blobs only, slash
// separated) and returns layout findings. read supplies file contents for
// the few files whose text decides a suggestion.
func Analyze(paths []string, read Reader) Result {
	res := Result{Tools: []string{}, Dockerfiles: []string{}, ComposeFiles: []string{}, Suggestions: []Suggestion{}}
	files := filterIgnored(paths)
	present := make(map[string]bool, len(files))
	for _, f := range files {
		present[f] = true
	}

	tools := workspaceTools(present, read)
	res.Tools = tools

	var dockerfiles []string
	for _, f := range files {
		switch {
		case isDockerfile(f):
			dockerfiles = append(dockerfiles, f)
		case isComposeFile(f):
			res.ComposeFiles = append(res.ComposeFiles, f)
		}
	}
	sort.Strings(dockerfiles)
	sort.Strings(res.ComposeFiles)
	if len(dockerfiles) > maxDockerfiles {
		dockerfiles = dockerfiles[:maxDockerfiles]
		res.Truncated = true
	}
	res.Dockerfiles = append(res.Dockerfiles, dockerfiles...)

	workspaceRoot := len(tools) > 0
	res.RootHasApp = rootHasApp(present, workspaceRoot)
	appDirs := appDirectories(files)
	res.LooksLikeMonorepo = workspaceRoot || (!res.RootHasApp && (len(appDirs) >= 2 || hasNestedOnly(dockerfiles)))

	for _, df := range dockerfiles {
		res.Suggestions = append(res.Suggestions, suggestForDockerfile(df, workspaceRoot, read))
	}
	if res.LooksLikeMonorepo && len(dockerfiles) == 0 {
		for i, dir := range appDirs {
			if i >= maxAppDirs {
				break
			}
			res.Suggestions = append(res.Suggestions, Suggestion{
				BuildType: buildTypeRailpack, BaseDirectory: dir, ReasonCode: ReasonAppRailpack, Score: 30,
				Reason: dir + " looks like an application; auto-detect it from that directory.",
			})
		}
	}

	sort.SliceStable(res.Suggestions, func(i, j int) bool {
		if res.Suggestions[i].Score != res.Suggestions[j].Score {
			return res.Suggestions[i].Score > res.Suggestions[j].Score
		}
		return res.Suggestions[i].DockerfilePath+res.Suggestions[i].BaseDirectory < res.Suggestions[j].DockerfilePath+res.Suggestions[j].BaseDirectory
	})
	if len(res.Suggestions) > maxSuggestions {
		res.Suggestions = res.Suggestions[:maxSuggestions]
	}
	if len(res.Suggestions) > 0 {
		res.Suggestions[0].Recommended = true
	}
	return res
}

func suggestForDockerfile(df string, workspaceRoot bool, read Reader) Suggestion {
	dir := path.Dir(df)
	if dir == "." {
		return Suggestion{
			BuildType: buildTypeDockerfile, DockerfilePath: df, ReasonCode: ReasonRootDockerfile, Score: 90,
			Reason: "Dockerfile at the repository root; build with the whole repository as context.",
		}
	}
	text := ""
	if read != nil {
		if b, err := read(df, 32*1024); err == nil {
			text = string(b)
		}
	}
	switch {
	case strings.Contains(text, "turbo prune"):
		return Suggestion{
			BuildType: buildTypeDockerfile, DockerfilePath: df, ReasonCode: ReasonTurboPrune, Score: 100,
			Reason: "Dockerfile at " + df + "; it runs turbo prune, so keep the build context at the repository root.",
		}
	case copiesWholeRepo(text, workspaceRoot):
		return Suggestion{
			BuildType: buildTypeDockerfile, DockerfilePath: df, ReasonCode: ReasonRootCopy, Score: 95,
			Reason: "Dockerfile at " + df + "; it copies the whole repository and workspace files, so keep the build context at the repository root.",
		}
	}
	if appDir, ok := appDirOfDeployDockerfile(df); ok {
		return Suggestion{
			BuildType: buildTypeDockerfile, DockerfilePath: df, ReasonCode: ReasonAppDeployDir, Score: 80,
			Reason: "Dockerfile in the deploy directory of " + appDir + "; apps like this usually build from the repository root.",
		}
	}
	return Suggestion{
		BuildType: buildTypeDockerfile, DockerfilePath: df, BaseDirectory: dir, ReasonCode: ReasonNestedDocker, Score: 50,
		Reason: "Dockerfile in " + dir + "; build from that directory.",
	}
}

func copiesWholeRepo(text string, workspaceRoot bool) bool {
	if text == "" {
		return false
	}
	for _, marker := range []string{"pnpm-workspace.yaml", "turbo.json", "nx.json", "go.work", "lerna.json"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	if !workspaceRoot {
		return false
	}
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) >= 3 && strings.EqualFold(f[0], "COPY") && f[len(f)-2] == "." && f[len(f)-1] == "." {
			return true
		}
	}
	return false
}

func appDirOfDeployDockerfile(df string) (string, bool) {
	dir := path.Dir(df)
	if !deployDirs[path.Base(dir)] {
		return "", false
	}
	app := path.Dir(dir)
	if app == "." {
		return "", false
	}
	return app, true
}

func workspaceTools(present map[string]bool, read Reader) []string {
	var tools []string
	add := func(cond bool, name string) {
		if cond {
			tools = append(tools, name)
		}
	}
	add(present["turbo.json"], "turbo")
	add(present["pnpm-workspace.yaml"], "pnpm-workspace")
	add(present["nx.json"], "nx")
	add(present["lerna.json"], "lerna")
	add(present["go.work"], "go-work")
	if present["package.json"] && read != nil {
		if b, err := read("package.json", 64*1024); err == nil && strings.Contains(string(b), `"workspaces"`) {
			tools = append(tools, "npm-workspaces")
		}
	}
	if present["Cargo.toml"] && read != nil {
		if b, err := read("Cargo.toml", 64*1024); err == nil && strings.Contains(string(b), "[workspace]") {
			tools = append(tools, "cargo-workspace")
		}
	}
	return tools
}

func rootHasApp(present map[string]bool, workspaceRoot bool) bool {
	if present["Dockerfile"] {
		return true
	}
	if workspaceRoot {
		return false
	}
	for _, f := range rootAppFiles {
		if present[f] {
			return true
		}
	}
	return present["package.json"] || present["Cargo.toml"]
}

func appDirectories(files []string) []string {
	seen := map[string]bool{}
	for _, f := range files {
		parts := strings.Split(f, "/")
		if len(parts) != 3 || !appParentDirs[parts[0]] {
			continue
		}
		switch parts[2] {
		case "package.json", "go.mod", "requirements.txt", "pyproject.toml", "Cargo.toml", "pom.xml":
			seen[parts[0]+"/"+parts[1]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func hasNestedOnly(dockerfiles []string) bool {
	if len(dockerfiles) == 0 {
		return false
	}
	for _, d := range dockerfiles {
		if path.Dir(d) == "." {
			return false
		}
	}
	return true
}

func filterIgnored(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimPrefix(p, "./")
		skip := false
		for _, seg := range strings.Split(p, "/") {
			if ignoredDirs[seg] {
				skip = true
				break
			}
		}
		if !skip && p != "" {
			out = append(out, p)
		}
	}
	return out
}

func isDockerfile(p string) bool {
	b := path.Base(p)
	if strings.HasSuffix(b, ".dockerignore") {
		return false
	}
	lb := strings.ToLower(b)
	return b == "Dockerfile" || strings.HasPrefix(b, "Dockerfile.") || strings.HasSuffix(lb, ".dockerfile") || lb == "containerfile"
}

func isComposeFile(p string) bool {
	b := strings.ToLower(path.Base(p))
	for _, prefix := range []string{"docker-compose", "compose."} {
		if strings.HasPrefix(b, prefix) && (strings.HasSuffix(b, ".yml") || strings.HasSuffix(b, ".yaml")) {
			return true
		}
	}
	return false
}
