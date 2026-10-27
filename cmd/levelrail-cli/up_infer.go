package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/kit/stackdetect"
)

// Where each inferred value came from, printed so the operator knows what to override.
const (
	srcFlag     = "flag"
	srcSpec     = "app spec"
	srcDetected = "detected"
	srcGit      = "git remote"
	srcDefault  = "default"
)

// upFlags are the values `up` takes from the command line; zero means unset.
type upFlags struct {
	name, domain, repo, ref, strategy, memory string
	port, replicas                            int
	cpu                                       float64
	with                                      []string
	secrets                                   map[string]string
	upload                                    bool
}

// upDatabase is one --with entry. An empty Version means the server's default for the engine.
type upDatabase struct {
	Name, Engine, Version, EnvVar string
}

// upInference is everything `up` decided, plus where each value came from.
type upInference struct {
	Dir       string
	Stack     stackdetect.Stack
	SpecPath  string
	Flags     createFlags
	FileSpec  *spec.Spec
	Git       detectedGit
	Static    bool
	Domain    string
	Resources *serviceResources
	DBs       []upDatabase
	Sources   map[string]string
	Notes     []string
}

var dbNamePattern = regexp.MustCompile(`[^a-z0-9-]+`)

// Env var each engine's first database is injected as; later ones use the derived default.
var engineEnvVars = map[string]string{
	engineParamPostgres: "DATABASE_URL", engineParamMySQL: "DATABASE_URL", engineParamMariaDB: "DATABASE_URL",
	engineParamRedis: "REDIS_URL", engineParamValkey: "REDIS_URL", engineParamKeyDB: "REDIS_URL", engineParamDragonfly: "REDIS_URL",
	engineParamMongoDB: "MONGODB_URL",
}

// inferUp resolves one app from flags, the app spec, stack detection and defaults, in that order.
func inferUp(dir string, f upFlags, git gitSource) (upInference, error) {
	inf := upInference{Dir: dir, Sources: map[string]string{}}
	stack, err := stackdetect.Detect(dir)
	if err != nil {
		return inf, fmt.Errorf("detect stack in %s: %w", dir, err)
	}
	inf.Stack = stack
	inf.Git = detectedGit{RepoURL: git.RepoURL, Ref: git.Ref}

	if path, derr := spec.DiscoverPath(dir, ""); derr == nil {
		if err := inf.applySpec(path, f, git); err != nil {
			return inf, err
		}
	} else if err := inf.applyDetection(f, git); err != nil {
		return inf, err
	}

	inf.pick("repo", f.repo, git.RepoURL, srcGit, &inf.Flags.repo)
	inf.pick("ref", f.ref, git.Ref, srcGit, &inf.Flags.ref)
	if inf.Flags.ref == "" {
		inf.Flags.ref, inf.Sources["ref"] = "main", srcDefault
	}
	if inf.Flags.repo == "" && !f.upload {
		return inf, newValidationError("no git remote \"origin\" found in %s, so the server has nothing to build from; add one (git remote add origin URL) and push, pass --repo URL, or upload the folder without git: levelrail-cli deploy %s", dir, dir)
	}
	if !inf.Static {
		inf.Flags.imageRepo, inf.Sources["image repo"] = inf.Flags.name, srcDefault
	}
	inf.Flags.secrets = f.secrets

	inf.applyOverrides(f)
	if err := inf.parseResources(f); err != nil {
		return inf, err
	}
	return inf, inf.parseDatabases(f)
}

func (inf *upInference) pick(label, flagVal, found, foundSrc string, dst *string) {
	switch {
	case flagVal != "":
		*dst, inf.Sources[label] = flagVal, srcFlag
	case found != "":
		*dst, inf.Sources[label] = found, foundSrc
	}
}

func (inf *upInference) applySpec(path string, f upFlags, git gitSource) error {
	data, err := os.ReadFile(path) //nolint:gosec // the project directory the operator pointed at
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	parsed, err := spec.Parse(data)
	if err != nil {
		return newValidationError("parse %s: %v", path, err)
	}
	if len(parsed.Services) != 1 {
		return newValidationError("%s declares %d services; up handles one app, use \"apps create --file %s --service KEY\" for the others", filepath.Base(path), len(parsed.Services), filepath.Base(path))
	}
	var key string
	var svc spec.Service
	for k, v := range parsed.Services {
		key, svc = k, v
	}
	switch svc.Build.Type {
	case spec.BuildDockerfile, spec.BuildRailpack, spec.BuildStatic:
	default:
		return newValidationError("%s has build.type %q; up builds from git (dockerfile, railpack or static), use \"deploy NAME --image IMAGE\" for a prebuilt image", filepath.Base(path), svc.Build.Type)
	}
	inf.SpecPath, inf.FileSpec = path, parsed
	inf.Static = svc.Build.Type == spec.BuildStatic
	inf.Flags = createFlags{file: path, service: key}
	inf.Sources["build"] = srcSpec
	inf.Notes = append(inf.Notes, fmt.Sprintf("using %s", filepath.Base(path)))
	if len(parsed.Databases) > 0 {
		inf.Notes = append(inf.Notes, "databases declared in the spec are not created by up; add them with --with or apply the spec")
	}

	inf.pick("name", f.name, key, srcSpec, &inf.Flags.name)
	inf.Flags.port, inf.Sources["port"] = firstPort(f.port, svc.Port, inf.Stack.Port, inf.Static)
	if svc.Build.BaseDirectory == "" {
		inf.Flags.baseDirectory = git.RelDir
	}
	if len(svc.Domains) > 0 {
		inf.Domain, inf.Sources["domain"] = svc.Domains[0], srcSpec
	}
	inf.Flags.replicas, inf.Flags.strategy = svc.Replicas, svc.Strategy
	if svc.Replicas > 0 {
		inf.Sources["replicas"] = srcSpec
	}
	if svc.Strategy != "" {
		inf.Sources["strategy"] = srcSpec
	}
	if svc.Resources != nil {
		res, err := toServiceResources(svc.Resources)
		if err != nil {
			return newValidationError("%s resources: %v", filepath.Base(path), err)
		}
		inf.Resources, inf.Sources["resources"] = res, srcSpec
	}
	return nil
}

func firstPort(flagPort, specPort, detected int, static bool) (int, string) {
	switch {
	case static:
		return 0, ""
	case flagPort > 0:
		return flagPort, srcFlag
	case specPort > 0:
		return specPort, srcSpec
	case detected > 0:
		return detected, srcDetected
	}
	return 0, ""
}

func (inf *upInference) applyDetection(f upFlags, git gitSource) error {
	st := inf.Stack
	switch {
	case !st.Detected():
		return newValidationError("could not tell how to build %s: add a Dockerfile, a package.json, go.mod or similar, or an app.yaml", inf.Dir)
	case st.Build == stackdetect.BuildCompose:
		return newValidationError("%s is a Docker Compose project; deploy it with \"compose deploy\" or \"apps deploy-compose\"", inf.Dir)
	}
	inf.Sources["build"] = srcDetected
	inf.Notes = append(inf.Notes, "detected "+st.Label)
	inf.Notes = append(inf.Notes, st.Notes...)
	inf.Static = st.Build == stackdetect.BuildStatic
	inf.Flags = createFlags{buildType: st.Build, baseDirectory: git.RelDir}
	if st.Build == stackdetect.BuildDockerfile && st.Path != "./Dockerfile" {
		inf.Flags.dockerfile = strings.TrimPrefix(st.Path, "./")
	}
	if inf.Static {
		inf.FileSpec = &spec.Spec{Version: 1, Services: map[string]spec.Service{
			st.Name: {Build: spec.Build{Type: spec.BuildStatic, Path: st.Path}},
		}}
		inf.Flags.file, inf.Flags.service = "(detected)", st.Name
	}
	inf.Flags.name, inf.Sources["name"] = st.Name, srcDetected
	if f.name != "" {
		inf.Flags.name, inf.Sources["name"] = f.name, srcFlag
	}
	inf.Flags.port, inf.Sources["port"] = firstPort(f.port, 0, st.Port, inf.Static)
	if inf.Flags.port == 0 && !inf.Static {
		return newValidationError("no port found for %s, pass --port N", inf.Dir)
	}
	return nil
}

func (inf *upInference) applyOverrides(f upFlags) {
	inf.Flags.replicas, inf.Sources["replicas"] = pickInt(f.replicas, inf.Flags.replicas, inf.Sources["replicas"])
	if f.strategy != "" {
		inf.Flags.strategy, inf.Sources["strategy"] = f.strategy, srcFlag
	}
	if inf.Flags.replicas == 0 {
		inf.Sources["replicas"] = srcDefault
	}
	if inf.Flags.strategy == "" {
		inf.Sources["strategy"] = srcDefault
	}
	if f.domain != "" {
		inf.Domain, inf.Sources["domain"] = strings.ToLower(f.domain), srcFlag
	}
	if inf.Domain == "" {
		inf.Sources["domain"] = srcDefault
	}
}

func pickInt(flagVal, cur int, curSrc string) (int, string) {
	if flagVal > 0 {
		return flagVal, srcFlag
	}
	return cur, curSrc
}

func (inf *upInference) parseResources(f upFlags) error {
	if f.memory == "" && f.cpu == 0 {
		if inf.Resources == nil {
			inf.Sources["resources"] = srcDefault
		}
		return nil
	}
	res := serviceResources{}
	if inf.Resources != nil {
		res = *inf.Resources
	}
	if f.memory != "" {
		b, err := parseMemoryBytes(f.memory)
		if err != nil {
			return newValidationError("--memory: %v", err)
		}
		res.MemoryBytes = b
	}
	if f.cpu < 0 {
		return newValidationError("--cpu must be positive, got %v", f.cpu)
	}
	if f.cpu > 0 {
		res.NanoCPUs = int64(f.cpu * 1e9)
	}
	inf.Resources, inf.Sources["resources"] = &res, srcFlag
	return nil
}

func (inf *upInference) parseDatabases(f upFlags) error {
	if len(f.with) > 0 && inf.Static {
		return newValidationError("--with needs a running container, and a static site has none")
	}
	used := map[string]bool{}
	seen := map[string]bool{}
	for _, item := range f.with {
		engine, version, _ := strings.Cut(strings.ToLower(strings.TrimSpace(item)), ":")
		if !isSupportedEngine(engine) {
			return newValidationError("--with %q: engine must be one of %s", item, strings.Join(supportedEngineParams, ", "))
		}
		if seen[engine] {
			continue
		}
		seen[engine] = true
		env := engineEnvVars[engine]
		if used[env] {
			env = ""
		}
		used[env] = true
		name := strings.Trim(dbNamePattern.ReplaceAllString(inf.Flags.name+"-"+engine, "-"), "-")
		inf.DBs = append(inf.DBs, upDatabase{Name: name, Engine: engine, Version: version, EnvVar: env})
	}
	return nil
}

func isSupportedEngine(engine string) bool {
	for _, e := range supportedEngineParams {
		if e == engine {
			return true
		}
	}
	return false
}

// upCommands is the exact existing-command expansion of the plan, one line per step.
func (inf upInference) upCommands(prog string, appExists bool) []string {
	f := inf.Flags
	var out []string
	switch {
	case inf.Static:
		out = append(out, fmt.Sprintf("%s apps deploy-spec %s --file %s --repo-url %s --ref %s", prog, f.name, filepath.Base(inf.specFile()), f.repo, f.ref))
	case appExists:
		out = append(out, fmt.Sprintf("%s apps builds trigger %s --repo %s --ref %s", prog, f.name, f.repo, f.ref))
	default:
		out = append(out, inf.createCommand(prog))
	}
	if inf.Resources != nil {
		out = append(out, inf.scaleCommand(prog))
	}
	for _, db := range inf.DBs {
		version := db.Version
		if version == "" {
			version = "<engine default>"
		}
		out = append(out, fmt.Sprintf("%s databases create --name %s --engine %s --version %s", prog, db.Name, db.Engine, version))
		connect := fmt.Sprintf("%s apps connect %s %s", prog, f.name, db.Name)
		if db.EnvVar != "" {
			connect += " --env-var " + db.EnvVar
		}
		out = append(out, connect)
	}
	if inf.Domain != "" {
		out = append(out, fmt.Sprintf("%s apps domains add %s %s", prog, f.name, inf.Domain))
	}
	return out
}

func (inf upInference) specFile() string {
	if inf.SpecPath != "" {
		return inf.SpecPath
	}
	return "app.yaml"
}

func (inf upInference) createCommand(prog string) string {
	f := inf.Flags
	var b strings.Builder
	fmt.Fprintf(&b, "%s apps create --name %s --port %d --repo %s --ref %s --image-repo %s", prog, f.name, f.port, f.repo, f.ref, f.imageRepo)
	if f.file != "" {
		fmt.Fprintf(&b, " --file %s", filepath.Base(f.file))
	} else {
		fmt.Fprintf(&b, " --build-type %s", f.buildType)
	}
	if f.baseDirectory != "" {
		fmt.Fprintf(&b, " --base-directory %s", f.baseDirectory)
	}
	if f.dockerfile != "" {
		fmt.Fprintf(&b, " --dockerfile %s", f.dockerfile)
	}
	if f.replicas > 0 {
		fmt.Fprintf(&b, " --replicas %d", f.replicas)
	}
	if f.strategy != "" {
		fmt.Fprintf(&b, " --strategy %s", f.strategy)
	}
	return b.String()
}

func (inf upInference) scaleCommand(prog string) string {
	cmd := fmt.Sprintf("%s apps scale %s", prog, inf.Flags.name)
	if inf.Resources.MemoryBytes > 0 {
		cmd += " --memory " + formatMemoryMi(inf.Resources.MemoryBytes)
	}
	if inf.Resources.NanoCPUs > 0 {
		cmd += " --cpu " + strconv.FormatFloat(float64(inf.Resources.NanoCPUs)/1e9, 'f', -1, 64)
	}
	return cmd
}

func formatMemoryMi(b int64) string {
	const mi, gi = 1 << 20, 1 << 30
	if b%gi == 0 {
		return fmt.Sprintf("%dGi", b/gi)
	}
	return fmt.Sprintf("%dMi", b/mi)
}
