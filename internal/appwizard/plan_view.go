package appwizard

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// Warning codes added by the plan view on top of the analysis ones.
const (
	WarnNoDockerfile = "no_dockerfile"
	WarnSpecDraft    = "spec_draft_invalid"
)

// PlanNext points at the existing action that applies a reviewed plan. The
// plan itself never applies anything.
type PlanNext struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// PlanStack is the detected stack and the files or signals behind it.
type PlanStack struct {
	Runtime    string   `json:"runtime"`
	Framework  string   `json:"framework,omitempty"`
	Language   string   `json:"language,omitempty"`
	Confidence string   `json:"confidence"`
	Evidence   []Reason `json:"evidence"`
}

// PlanBuild is how the app would be built and started.
type PlanBuild struct {
	Type           string `json:"type"`
	DockerfilePath string `json:"dockerfile_path,omitempty"`
	BaseDirectory  string `json:"base_directory,omitempty"`
	BuildCommand   string `json:"build_command,omitempty"`
	StartCommand   string `json:"start_command,omitempty"`
	OutputDir      string `json:"output_dir,omitempty"`
	Evidence       string `json:"evidence"`
}

// PlanEnv is one env var name the repository documents. It has no value
// field on purpose: values are never read into the plan.
type PlanEnv struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Required    bool   `json:"required"`
	LooksSecret bool   `json:"looks_secret"`
	HasDefault  bool   `json:"has_default_hint"`
}

// PlanSpec is the draft app spec in the declarative spec format.
type PlanSpec struct {
	Name string `json:"name"`
	YAML string `json:"yaml"`
}

// PlanPort is a guessed port or health path with its evidence.
type PlanPort struct {
	Value    int    `json:"value"`
	Evidence string `json:"evidence"`
}

// PlanHealth is the guessed health probe path.
type PlanHealth struct {
	Path     string `json:"path"`
	Evidence string `json:"evidence"`
}

// PlanView is the read-only, reviewable plan for one repository.
type PlanView struct {
	RepoURL      string     `json:"repo_url"`
	Ref          string     `json:"ref"`
	AppDirectory string     `json:"app_directory,omitempty"`
	Monorepo     bool       `json:"monorepo"`
	Stack        PlanStack  `json:"stack"`
	Build        PlanBuild  `json:"build"`
	Port         PlanPort   `json:"port"`
	Health       PlanHealth `json:"health"`
	Env          []PlanEnv  `json:"env"`
	Spec         PlanSpec   `json:"spec"`
	Warnings     []Warning  `json:"warnings"`
	Next         PlanNext   `json:"next"`
}

type draftSpec struct {
	Version  int                     `yaml:"version"`
	Services map[string]draftService `yaml:"services"`
}

type draftService struct {
	Build  spec.Build          `yaml:"build"`
	Port   int                 `yaml:"port,omitempty"`
	Health *draftHealth        `yaml:"health,omitempty"`
	Env    map[string]draftEnv `yaml:"env,omitempty"`
}

type draftHealth struct {
	Readiness struct {
		Path string `yaml:"path"`
	} `yaml:"readiness"`
}

// draftEnv is the spec form of an env var that has no known value: a
// secret the operator supplies at deploy time.
type draftEnv struct {
	Secret   bool `yaml:"secret"`
	Required bool `yaml:"required,omitempty"`
}

// BuildPlanView turns an analysis into a reviewable plan. next is the
// existing apply action to link to.
func BuildPlanView(a *Analysis, next PlanNext) PlanView {
	d := a.Detection
	ev := append([]Reason{}, d.Reasons...)
	if a.Stack.Evidence != "" {
		ev = append([]Reason{{Signal: "stack", Detail: a.Stack.Evidence}}, ev...)
	}
	pv := PlanView{
		RepoURL: a.RepoURL, Ref: a.Branch, AppDirectory: a.AppDirectory, Monorepo: a.Monorepo,
		Stack: PlanStack{Runtime: a.Stack.Runtime, Framework: firstNonEmpty(d.Framework, a.Stack.Framework), Language: d.Language, Confidence: d.Confidence, Evidence: ev},
		Build: PlanBuild{
			Type: a.Build.Type, DockerfilePath: a.Build.DockerfilePath, BaseDirectory: a.Build.BaseDirectory,
			BuildCommand: d.BuildCommand, StartCommand: d.StartCommand, OutputDir: d.OutputDir, Evidence: a.Build.Evidence,
		},
		Port:   PlanPort{Value: a.Port.Value, Evidence: a.Port.Evidence},
		Health: PlanHealth{Path: a.Health.Value, Evidence: a.Health.Evidence},
		Env:    make([]PlanEnv, 0, len(a.Env)),
		Next:   next,
	}
	for _, k := range a.Env {
		pv.Env = append(pv.Env, PlanEnv{Name: k.Key, Source: k.Source, Required: k.Required, LooksSecret: k.Secret, HasDefault: k.HasDefault})
	}
	pv.Warnings = mergeWarnings(a.Warnings, d.Warnings)
	if a.Build.Type != buildDockerfile {
		pv.Warnings = appendWarning(pv.Warnings, WarnNoDockerfile, "no Dockerfile is used, so the build is auto-detected; add a Dockerfile for full control")
	}
	if a.Monorepo {
		pv.Warnings = appendWarning(pv.Warnings, WarnMonorepo, "this looks like a monorepo; check the suggested app directory before applying")
	}
	pv.Spec = draftFor(a, pv.Build, &pv.Warnings)
	return pv
}

func draftFor(a *Analysis, b PlanBuild, warns *[]Warning) PlanSpec {
	name := a.Name.Value
	if name == "" {
		name = "app"
	}
	sb, err := spec.ResolveGitBuild(b.Type, b.BaseDirectory, b.DockerfilePath)
	if err != nil {
		*warns = appendWarning(*warns, WarnSpecDraft, "the build settings do not form a valid spec: "+err.Error())
		return PlanSpec{Name: name}
	}
	if b.Type == buildStatic {
		if b.BuildCommand != "" {
			sb.Command, sb.Publish = b.BuildCommand, b.OutputDir
		} else if out, perr := spec.NormalizeRepoPath("output", b.OutputDir); perr == nil {
			sb.Path = out
		}
	}
	svc := draftService{Build: sb, Port: a.Port.Value}
	if b.Type == buildStatic {
		svc.Port = 0
	}
	if b.Type != buildStatic && a.Health.Value != "" {
		svc.Health = &draftHealth{}
		svc.Health.Readiness.Path = a.Health.Value
	}
	// Only names the operator must supply are drafted; values are never known here.
	for _, k := range a.Env {
		if k.Secret || k.Required {
			if svc.Env == nil {
				svc.Env = map[string]draftEnv{}
			}
			svc.Env[k.Key] = draftEnv{Secret: true, Required: k.Required}
		}
	}
	out, err := yaml.Marshal(draftSpec{Version: 1, Services: map[string]draftService{name: svc}})
	if err != nil {
		*warns = appendWarning(*warns, WarnSpecDraft, "the draft spec could not be rendered")
		return PlanSpec{Name: name}
	}
	if _, err := spec.Parse(out); err != nil {
		*warns = appendWarning(*warns, WarnSpecDraft, "the draft spec needs edits before it validates: "+err.Error())
	}
	return PlanSpec{Name: name, YAML: string(out)}
}

func mergeWarnings(lists ...[]Warning) []Warning {
	out := []Warning{}
	seen := map[string]bool{}
	for _, l := range lists {
		for _, w := range l {
			if seen[w.Code] {
				continue
			}
			seen[w.Code] = true
			out = append(out, w)
		}
	}
	return out
}

func appendWarning(ws []Warning, code, msg string) []Warning {
	for _, w := range ws {
		if w.Code == code {
			return ws
		}
	}
	return append(ws, Warning{Code: code, Message: msg})
}

// skippedDirs are never listed: they are vendored or VCS data.
var skippedDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true}

// realEnvFile reports whether a file name is a real env file. Only the
// documented example forms may be read.
func realEnvFile(name string) bool {
	if name != ".env" && !strings.HasPrefix(name, ".env.") {
		return false
	}
	for _, ok := range []string{".env.example", ".env.sample", ".env.template", ".env.dist"} {
		if name == ok {
			return false
		}
	}
	return true
}

// DirSource lists and reads an already checked out source directory
// read-only. Real env files, symlinks and paths outside root are never
// listed or read.
type DirSource struct{ Root string }

// Paths lists regular files below Root, sorted, relative with slashes.
func (d DirSource) Paths(ctx context.Context) ([]string, bool, error) {
	var out []string
	err := filepath.WalkDir(d.Root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if e.IsDir() {
			if p != d.Root && skippedDirs[e.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !e.Type().IsRegular() || realEnvFile(e.Name()) {
			return nil
		}
		rel, rerr := filepath.Rel(d.Root, p)
		if rerr != nil {
			return rerr
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out, false, err
}

// Read returns at most limit bytes of one listed file.
func (d DirSource) Read(_ context.Context, p string, limit int) ([]byte, error) {
	clean := filepath.Clean(filepath.FromSlash(p))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || realEnvFile(filepath.Base(clean)) {
		return nil, errors.New("path not readable")
	}
	full := filepath.Join(d.Root, clean)
	if !d.insideRoot(full) {
		return nil, errors.New("path not readable")
	}
	f, err := os.Open(full) //nolint:gosec // full is confined to Root by insideRoot
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, limit)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return []byte{}, nil
	}
	return buf[:n], nil
}

// insideRoot resolves symlinks in every component so a linked directory
// cannot lead outside Root, and requires a regular file.
func (d DirSource) insideRoot(full string) bool {
	root, err := filepath.EvalSymlinks(d.Root)
	if err != nil {
		return false
	}
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil || (resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator))) {
		return false
	}
	fi, err := os.Stat(resolved)
	return err == nil && fi.Mode().IsRegular()
}
