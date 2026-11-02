package appwizard

import (
	"path"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/importplan"
	"github.com/GLINCKER/levelrail/internal/repolayout"
)

// Confidence levels of a Detection.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// Build types a Detection can name.
const buildCompose = "compose"

// Detection warning codes.
const (
	WarnNoLockfile        = "no_lockfile"
	WarnMultiDockerfile   = "multiple_dockerfiles"
	WarnMalformedManifest = "malformed_manifest"
	WarnDockerfileNoPort  = "dockerfile_no_expose"
	WarnMonorepoNoApp     = "monorepo_no_root_app"
	WarnRailpackNoRust    = "railpack_unsupported"
)

const maxManifestBytes = 1 << 20

// Reason is one signal that led to a detected value.
type Reason struct {
	Signal string `json:"signal"`
	Detail string `json:"detail"`
}

// Sourced is a value with the place it came from.
type Sourced struct {
	Value  string `json:"value"`
	Source string `json:"source"`
}

// PortGuess is the listening port and where it came from.
type PortGuess struct {
	Value  int    `json:"value"`
	Source string `json:"source"`
}

// EnvVarGuess is one variable the repository declares, never with a value.
type EnvVarGuess struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Required    bool   `json:"required"`
	SecretGuess bool   `json:"secret_guess"`
}

// Detection explains what a repository is and how it would be built.
type Detection struct {
	Framework    string    `json:"framework"`
	Language     string    `json:"language"`
	BuildType    string    `json:"build_type"`
	Confidence   string    `json:"confidence"`
	Reasons      []Reason  `json:"reasons"`
	Port         PortGuess `json:"port"`
	StartCommand string    `json:"start_command,omitempty"`
	BuildCommand string    `json:"build_command,omitempty"`
	OutputDir    string    `json:"output_dir,omitempty"`
	// SPA is true for a client routed single page app: serve index.html for unknown paths.
	SPA            bool          `json:"spa"`
	HealthPath     string        `json:"health_path"`
	RuntimeVersion Sourced       `json:"runtime_version"`
	EnvVars        []EnvVarGuess `json:"env_vars"`
	Warnings       []Warning     `json:"warnings"`
}

// Input is a repository tree, from a checkout or a provider listing. Read
// returns false for anything missing, unreadable or binary.
type Input struct {
	Paths []string
	Read  func(p string) (string, bool)
	// Dir is the app directory inside the repository, "" for the root.
	Dir    string
	Layout *repolayout.Result
}

type files struct {
	dir     string
	present map[string]bool
	read    func(string) (string, bool)
	cache   map[string]string
	bad     []string
}

func newFiles(in Input) *files {
	f := &files{dir: strings.Trim(in.Dir, "/"), present: make(map[string]bool, len(in.Paths)), read: in.Read, cache: map[string]string{}}
	for _, p := range in.Paths {
		f.present[strings.TrimPrefix(p, "./")] = true
	}
	return f
}

func (f *files) full(rel string) string {
	if f.dir == "" {
		return rel
	}
	return path.Join(f.dir, rel)
}

func (f *files) has(rel string) bool { return f.present[f.full(rel)] }

func (f *files) anyUnder(prefix string) bool {
	p := f.full(prefix) + "/"
	for k := range f.present {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

func (f *files) text(rel string) (string, bool) {
	full := f.full(rel)
	if !f.present[full] || f.read == nil {
		return "", false
	}
	if s, ok := f.cache[full]; ok {
		return s, true
	}
	s, ok := f.read(full)
	if !ok || strings.ContainsRune(s, 0) {
		return "", false
	}
	if len(s) > maxManifestBytes {
		s = s[:maxManifestBytes]
	}
	f.cache[full] = s
	return s, true
}

func (f *files) anyOf(names ...string) (string, bool) {
	for _, n := range names {
		if f.has(n) {
			return n, true
		}
	}
	return "", false
}

type found struct {
	framework, language, buildType    string
	confidence                        string
	port                              PortGuess
	start, build, output, health      string
	version                           Sourced
	spa                               bool
	reasons                           []Reason
	noLockfile, malformed, noManifest bool
}

func (g *found) why(signal, detail string) { g.reasons = append(g.reasons, Reason{signal, detail}) }

// Detect names the framework, build type and run settings of a repository
// and lists every signal behind each choice. It is pure: no network, no clone.
func Detect(in Input) Detection {
	f := newFiles(in)
	g := detectManifest(f)
	d := Detection{Warnings: []Warning{}, EnvVars: []EnvVarGuess{}, Reasons: []Reason{}}

	dockerfiles := dockerfilesIn(in.Paths, f.dir)
	switch {
	case len(dockerfiles) > 0:
		applyDockerfile(f, dockerfiles, &g, &d)
	case g.framework == "" && composeIn(f):
		g.buildType, g.confidence = buildCompose, ConfidenceMedium
		g.why("file", "a compose file is present and no Dockerfile or framework manifest was found")
	}

	d.Framework, d.Language, d.BuildType = g.framework, g.language, g.buildType
	d.StartCommand, d.BuildCommand, d.OutputDir = g.start, g.build, g.output
	d.SPA = g.spa && g.buildType == buildStatic
	d.Reasons = append(d.Reasons, g.reasons...)
	d.RuntimeVersion = g.version
	d.Port = g.port
	d.HealthPath = g.health
	if d.HealthPath == "" && d.BuildType != buildCompose {
		d.HealthPath = "/"
	}
	if d.BuildType == "" {
		d.BuildType = buildRailpack
		d.Confidence = ConfidenceLow
		d.Warnings = append(d.Warnings, Warning{Code: WarnNothingDetected, Message: "no Dockerfile, compose file or known framework was found; pick the build type manually"})
		d.Reasons = append(d.Reasons, Reason{"none", "no recognised manifest, Dockerfile or index.html"})
	} else {
		d.Confidence = g.confidence
	}
	if d.Port.Value == 0 && d.BuildType != buildStatic && d.BuildType != buildCompose {
		d.Warnings = append(d.Warnings, Warning{Code: WarnNoPort, Message: "no listening port could be detected; set the port the app listens on"})
	}
	if d.BuildType == buildRailpack && g.language == "rust" {
		d.Warnings = append(d.Warnings, Warning{Code: WarnRailpackNoRust, Message: "automatic builds do not cover Rust yet; add a Dockerfile to the repository"})
	}
	if g.noLockfile {
		d.Warnings = append(d.Warnings, Warning{Code: WarnNoLockfile, Message: "no lockfile was found, so installs are not reproducible; commit one"})
	}
	if g.malformed {
		d.Warnings = append(d.Warnings, Warning{Code: WarnMalformedManifest, Message: "a manifest could not be parsed; the result is a best guess from its raw text"})
		if d.Confidence == ConfidenceHigh {
			d.Confidence = ConfidenceMedium
		}
	}
	if in.Layout != nil && in.Layout.LooksLikeMonorepo && !in.Layout.RootHasApp && f.dir == "" {
		d.Warnings = append(d.Warnings, Warning{Code: WarnMonorepoNoApp, Message: "this is a monorepo with no app at the root; create one app per package"})
	}
	d.EnvVars = envGuesses(f)
	return d
}

func composeIn(f *files) bool {
	_, ok := f.anyOf("docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml")
	return ok
}

func dockerfilesIn(paths []string, dir string) []string {
	var out []string
	for _, p := range paths {
		base := path.Base(p)
		if base != "Dockerfile" && !strings.HasPrefix(base, "Dockerfile.") {
			continue
		}
		if dir != "" && !strings.HasPrefix(p, dir+"/") {
			continue
		}
		if ignoredDockerPath(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func ignoredDockerPath(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "node_modules", "vendor", ".git", "testdata", "fixtures":
			return true
		}
	}
	return false
}

// applyDockerfile makes a Dockerfile the build type, taking the shallowest
// one, and lets its EXPOSE and HEALTHCHECK override framework defaults.
func applyDockerfile(f *files, dockerfiles []string, g *found, d *Detection) {
	best := dockerfiles[0]
	for _, p := range dockerfiles {
		if strings.Count(p, "/") < strings.Count(best, "/") || (strings.Count(p, "/") == strings.Count(best, "/") && p < best) {
			best = p
		}
	}
	if len(dockerfiles) > 1 {
		d.Warnings = append(d.Warnings, Warning{Code: WarnMultiDockerfile, Message: "several Dockerfiles were found; using " + best + ", pick another if it is wrong"})
	}
	g.buildType = buildDockerfile
	g.why("dockerfile", best+" exists, so the Dockerfile is used instead of auto-detection")
	switch {
	case g.framework == "" || g.confidence == "":
		g.confidence = ConfidenceHigh
	case g.confidence == ConfidenceLow:
		g.confidence = ConfidenceMedium
	}
	g.start, g.build, g.output = "", "", ""
	text := ""
	if f.read != nil {
		if s, ok := f.read(best); ok && !strings.ContainsRune(s, 0) {
			text = s
		}
	}
	ports, _ := importplan.ParseDockerfile(text)
	switch {
	case len(ports) > 0:
		g.port = PortGuess{ports[0], "EXPOSE in " + best}
		g.why("expose", best+" has EXPOSE "+strconv.Itoa(ports[0]))
	case g.port.Value > 0:
		g.port.Source += " (the Dockerfile has no EXPOSE)"
		d.Warnings = append(d.Warnings, Warning{Code: WarnDockerfileNoPort, Message: "the Dockerfile has no EXPOSE; verify the port the app listens on"})
	default:
		d.Warnings = append(d.Warnings, Warning{Code: WarnDockerfileNoPort, Message: "the Dockerfile has no EXPOSE; set the port the app listens on"})
	}
	if p := healthFromDockerfile(text); p != "" {
		g.health = p
		g.why("healthcheck", "the Dockerfile HEALTHCHECK probes "+p)
	}
}

func envGuesses(f *files) []EnvVarGuess {
	dirs := []string{f.dir}
	if f.dir != "" {
		dirs = append(dirs, "")
	}
	rd := func(p string) (string, bool) {
		if f.read == nil || !f.present[p] {
			return "", false
		}
		s, ok := f.read(p)
		return s, ok && !strings.ContainsRune(s, 0)
	}
	out := []EnvVarGuess{}
	for _, k := range mergeKeys(defaultMaxEnvKeys, envFromExampleFiles(rd, dirs)) {
		out = append(out, EnvVarGuess{Name: k.Key, Source: k.Source, Required: k.Required, SecretGuess: k.Secret})
	}
	return out
}
