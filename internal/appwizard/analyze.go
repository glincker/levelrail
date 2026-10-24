package appwizard

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/importplan"
	"github.com/GLINCKER/levelrail/internal/repolayout"
)

const (
	defaultMaxFiles     = 24
	defaultMaxFileBytes = 256 * 1024
	defaultMaxEnvKeys   = 60
	buildDockerfile     = "dockerfile"
	buildRailpack       = "railpack"
	buildStatic         = "static"
	staticPort          = 80
	maxNameLen          = 40
)

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

// budget caps how many file bodies, and how many bytes of each, one
// analysis reads. Only paths present in the tree listing are fetched.
type budget struct {
	ctx       context.Context
	src       repolayout.Source
	present   map[string]bool
	left      int
	maxBytes  int
	cache     map[string]string
	read      int
	exhausted bool
}

func (b *budget) get(p string) (string, bool) {
	if !b.present[p] {
		return "", false
	}
	if s, ok := b.cache[p]; ok {
		return s, true
	}
	if b.left <= 0 {
		b.exhausted = true
		return "", false
	}
	b.left--
	data, err := b.src.Read(b.ctx, p, b.maxBytes)
	if err != nil {
		return "", false
	}
	b.read++
	s := string(data)
	b.cache[p] = s
	return s, true
}

func hasPathUnder(paths []string, dir string) bool {
	prefix := dir + "/"
	for _, p := range paths {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

func joinPath(dir, file string) string {
	if dir == "" || dir == "." {
		return file
	}
	return path.Join(dir, file)
}

// Analyze lists the repository tree once, reads a bounded set of manifest
// files, and returns suggestions with evidence. It never clones and never
// returns env values.
func Analyze(ctx context.Context, src repolayout.Source, opts Options) (*Analysis, error) {
	paths, truncated, err := src.Paths(ctx)
	if err != nil {
		return nil, fmt.Errorf("appwizard: list repository files: %w", err)
	}
	b := &budget{
		ctx: ctx, src: src, present: make(map[string]bool, len(paths)), cache: map[string]string{},
		left: envInt(EnvMaxFiles, defaultMaxFiles), maxBytes: envInt(EnvMaxFileBytes, defaultMaxFileBytes),
	}
	for _, p := range paths {
		b.present[p] = true
	}
	layout := repolayout.Analyze(paths, func(p string, _ int) ([]byte, error) {
		if s, ok := b.get(p); ok {
			return []byte(s), nil
		}
		return nil, os.ErrNotExist
	})

	a := &Analysis{RepoURL: opts.RepoURL, Branch: opts.Branch, Monorepo: layout.LooksLikeMonorepo, Warnings: []Warning{}, TreeTruncated: truncated || layout.Truncated}
	a.Build = chooseBuild(layout, opts)
	a.AppDirectory = appDirectory(a.Build)

	rel := func(p string) (string, bool) { return b.get(joinPath(a.AppDirectory, p)) }
	st, guess := detectStack(rel)
	a.Stack = st
	if a.Build.Type == buildRailpack && guess.Method == importplan.BuildStatic && a.Build.DockerfilePath == "" {
		a.Build.Type = buildStatic
		a.Build.Evidence = st.Evidence
	}
	if dir := strings.Trim(a.Build.BaseDirectory, "/"); dir != "" && !hasPathUnder(paths, dir) && !truncated {
		a.Warnings = append(a.Warnings, Warning{Code: WarnDirectoryMissing, Message: "the directory " + dir + " does not exist in this repository at this branch"})
	}
	if st.Runtime == runtimeUnknown && a.Build.Type != buildDockerfile {
		a.Warnings = append(a.Warnings, Warning{Code: WarnNothingDetected, Message: "no Dockerfile, compose file or known framework was found; pick the build type manually"})
	}

	var dockerfile string
	if a.Build.Type == buildDockerfile {
		dockerfile, _ = b.get(a.Build.DockerfilePath)
	}
	pkg, _ := rel("package.json")
	a.Port = choosePort(dockerfile, a.Build, pkg, guess)
	if a.Port.Value == 0 && a.Build.Type != buildStatic {
		a.Warnings = append(a.Warnings, Warning{Code: WarnNoPort, Message: "no listening port could be detected; set the port the app listens on"})
	}
	a.Health = chooseHealth(dockerfile, guess)
	a.Memory = memorySuggestion(st)

	a.Name = chooseName(opts, a)
	a.Domain = chooseDomain(opts, a.Name.Value)

	dirs := []string{a.AppDirectory}
	if a.AppDirectory != "" {
		dirs = append(dirs, "")
	}
	reader := func(p string) (string, bool) { return b.get(p) }
	example := envFromExampleFiles(reader, dirs)
	doc, cpath, hasCompose := parseCompose(reader, dirs)
	var composeKeys []EnvKey
	if hasCompose {
		composeKeys = envFromCompose(doc, cpath)
	}
	var readmeKeys []EnvKey
	limit := envInt(EnvMaxEnvKeys, defaultMaxEnvKeys)
	for _, d := range dirs {
		if text, ok := b.get(joinPath(d, sourceREADME)); ok {
			readmeKeys = envFromREADME(text, limit)
			break
		}
	}
	a.Env = mergeKeys(limit, example, composeKeys, readmeKeys)

	a.Databases = detectDatabases(a.Name.Value, dbSignals{keys: a.Env, depText: depText(rel), compose: doc, cpath: cpath}, opts.DefaultVersion)
	markProvided(a.Env, a.Databases)

	a.Detection = Detect(Input{Paths: paths, Read: reader, Dir: a.AppDirectory, Layout: &layout})
	a.Packages = layout.Packages
	if a.Packages == nil {
		a.Packages = []repolayout.Package{}
	}
	a.FilesRead = b.read
	if b.exhausted {
		a.Warnings = append(a.Warnings, Warning{Code: WarnReadBudget, Message: "the file read budget was reached, so some findings may be missing"})
	}
	if a.TreeTruncated {
		a.Warnings = append(a.Warnings, Warning{Code: WarnTruncated, Message: "the repository is large and its file listing was cut short"})
	}
	if layout.LooksLikeMonorepo && !layout.RootHasApp && opts.BaseDirectory == "" && opts.DockerfilePath == "" && len(a.Build.Alternatives) > 1 {
		a.Warnings = append(a.Warnings, Warning{Code: WarnMonorepo, Message: "this looks like a monorepo with several apps; check the suggested app directory"})
	}
	a.fillEmptySlices()
	return a, nil
}

// fillEmptySlices keeps nil slices out of the JSON so clients can iterate safely.
func (a *Analysis) fillEmptySlices() {
	if a.Env == nil {
		a.Env = []EnvKey{}
	}
	if a.Databases == nil {
		a.Databases = []DatabaseNeed{}
	}
	if a.Warnings == nil {
		a.Warnings = []Warning{}
	}
	if a.Build.Alternatives == nil {
		a.Build.Alternatives = []repolayout.Suggestion{}
	}
	if a.Detection.EnvVars == nil {
		a.Detection.EnvVars = []EnvVarGuess{}
	}
	if a.Detection.Warnings == nil {
		a.Detection.Warnings = []Warning{}
	}
	if a.Detection.Reasons == nil {
		a.Detection.Reasons = []Reason{}
	}
}

func chooseBuild(layout repolayout.Result, opts Options) Build {
	alts := layout.Suggestions
	if alts == nil {
		alts = []repolayout.Suggestion{}
	}
	if opts.BuildType != "" || opts.BaseDirectory != "" || opts.DockerfilePath != "" {
		t := opts.BuildType
		if t == "" {
			t = buildRailpack
			if opts.DockerfilePath != "" {
				t = buildDockerfile
			}
		}
		return Build{Type: t, DockerfilePath: opts.DockerfilePath, BaseDirectory: opts.BaseDirectory, Evidence: "set by you", Alternatives: alts}
	}
	if len(alts) > 0 {
		s := alts[0]
		ev := s.Reason
		if s.DockerfilePath != "" {
			ev = "found Dockerfile at " + s.DockerfilePath + ": " + s.Reason
		}
		return Build{Type: s.BuildType, DockerfilePath: s.DockerfilePath, BaseDirectory: s.BaseDirectory, Evidence: ev, Alternatives: alts}
	}
	return Build{Type: buildRailpack, Evidence: "no Dockerfile found, the framework is auto-detected", Alternatives: alts}
}

// appDirectory is the directory the app's own manifests live in.
func appDirectory(b Build) string {
	if b.BaseDirectory != "" {
		return b.BaseDirectory
	}
	if b.DockerfilePath == "" {
		return ""
	}
	dir := path.Dir(b.DockerfilePath)
	if dir == "." {
		return ""
	}
	switch path.Base(dir) {
	case "deploy", "docker", ".docker", "build", "deployment", "infra":
		if up := path.Dir(dir); up != "." {
			return up
		}
		return ""
	}
	return dir
}

func choosePort(dockerfile string, b Build, pkg string, g importplan.StackGuess) Port {
	if dockerfile != "" {
		if ports, _ := importplan.ParseDockerfile(dockerfile); len(ports) > 0 {
			return Port{Value: ports[0], Evidence: fmt.Sprintf("%s has EXPOSE %d", b.DockerfilePath, ports[0])}
		}
	}
	if n, why := portFromScripts(pkg); n > 0 {
		return Port{Value: n, Evidence: why}
	}
	if b.Type == buildStatic {
		return Port{Value: staticPort, Evidence: "static sites are served by the built-in static server"}
	}
	if g.Port > 0 {
		return Port{Value: g.Port, Evidence: fmt.Sprintf("%s default port", g.Name)}
	}
	return Port{}
}

func chooseHealth(dockerfile string, g importplan.StackGuess) Text {
	if p := healthFromDockerfile(dockerfile); p != "" {
		return Text{Value: p, Evidence: "Dockerfile HEALTHCHECK probes " + p}
	}
	if g.Health != "" {
		return Text{Value: g.Health, Evidence: fmt.Sprintf("%s default health path", g.Name)}
	}
	return Text{Value: "/", Evidence: "no framework hint, / is the safest probe"}
}

func chooseName(opts Options, a *Analysis) Text {
	if opts.Name != "" {
		return Text{Value: slug(opts.Name), Evidence: "set by you"}
	}
	repo := repoName(opts.RepoURL)
	name := slug(repo)
	ev := "derived from the repository name " + repo
	if a.Monorepo && a.AppDirectory != "" {
		name = slug(repo + "-" + path.Base(a.AppDirectory))
		ev = "repository name plus app directory " + a.AppDirectory
	}
	return Text{Value: name, Evidence: ev}
}

func chooseDomain(opts Options, name string) Text {
	if opts.AppsBaseDomain == "" || opts.DomainFor == nil {
		return Text{Evidence: "no apps base domain is configured, add a domain after the first deploy"}
	}
	host := opts.DomainFor(name, opts.AppsBaseDomain)
	if host == "" {
		return Text{Evidence: "no apps base domain is configured, add a domain after the first deploy"}
	}
	return Text{Value: host, Evidence: "<app>." + opts.AppsBaseDomain + " from the apps base domain"}
}

// depText concatenates dependency manifests, lower-cased, for token search.
func depText(read reader) string {
	var sb strings.Builder
	for _, f := range []string{"package.json", "requirements.txt", "pyproject.toml", "Pipfile", "go.mod", "Gemfile", "composer.json", "pom.xml", "mix.exs"} {
		if t, ok := read(f); ok {
			sb.WriteString("\n")
			sb.WriteString(strings.ToLower(t))
		}
	}
	return sb.String()
}

func repoName(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "app"
	}
	base := strings.TrimSuffix(path.Base(strings.TrimSuffix(u.Path, "/")), ".git")
	if base == "" || base == "." || base == "/" {
		return "app"
	}
	return base
}

func slug(s string) string {
	var sb strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			sb.WriteRune(r)
			dash = false
		default:
			if !dash && sb.Len() > 0 {
				sb.WriteByte('-')
				dash = true
			}
		}
	}
	out := strings.Trim(sb.String(), "-")
	if len(out) > maxNameLen {
		out = strings.Trim(out[:maxNameLen], "-")
	}
	if out == "" {
		return "app"
	}
	return out
}

// PackageAppName proposes an app name for one workspace package: the
// repository name plus the package directory's last segment.
func PackageAppName(repoURL, dir string) string {
	return slug(repoName(repoURL) + "-" + path.Base(dir))
}

// MaxAppNameLen is the longest name PackageAppName returns.
const MaxAppNameLen = maxNameLen
