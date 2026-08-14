package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// createFlags is the raw, unvalidated input to "apps create": exactly
// what flag.FlagSet parsed, no mode inference or defaulting applied yet.
// Kept separate from createPlan so planFromFlags (the actual decision
// logic) is a pure function over plain data, testable without a
// flag.FlagSet in the loop at all.
type createFlags struct {
	name       string
	image      string
	port       int
	repo       string
	ref        string
	dockerfile string
	imageRepo  string
	file       string
	service    string
	yes        bool
	jsonOut    bool
}

// createPlan is planFromFlags's output: exactly the HTTP requests
// runAppsCreate needs to make, already validated. Build is nil for the
// existing-image path, non-nil for the git-build path (whether reached
// via flags or --file).
type createPlan struct {
	CreateBody appResource
	Build      *buildTriggerRequest
}

// pendingImageTag is the placeholder image value POST /apps is sent for
// the git-build path (both --repo and --file modes): validateAppResource
// (internal/api/apps.go) requires a non-empty image at creation time,
// but a git-build app has no built image yet. deploy.Pipeline's own
// Deploy call (internal/deploy/deploy.go) overwrites this with the real
// built tag via SaveDesiredService as soon as POST .../builds succeeds,
// the same "full replace, not a partial update" semantics that endpoint
// already documents, so this placeholder never lingers past a
// successful first build.
func pendingImageTag(imageRepo string) string {
	return imageRepo + ":pending"
}

// planFromFlags decides which of the three creation paths f describes
// (existing-image, git-build via flags, or --file) and builds the exact
// request(s) runAppsCreate will send. Pure: fileSpec is an already-parsed
// app.yaml (nil when --file wasn't given) and detected is an
// already-computed best-effort local git auto-detection result, so this
// function does no I/O itself and is fully covered by table-driven
// tests.
//
// An explicit --repo is not the only way into the git-build-via-flags
// path: a caller with no --file, no --image, and no --repo, standing in
// a directory detectLocalGit can read a remote from, also selects this
// path (detected.RepoURL != ""), so "cd into a checkout and run apps
// create --name x --port y --image-repo z" works without a redundant
// --repo, the same auto-detection the flags' own usage text
// (appsCreateUsage) already promises.
func planFromFlags(f createFlags, fileSpec *spec.Spec, detected detectedGit) (createPlan, error) {
	switch {
	case f.file != "":
		if f.image != "" {
			return createPlan{}, newValidationError("--image cannot be combined with --file: app.yaml has no image field, only a build declaration")
		}
		return planFromFile(f, fileSpec, detected)
	case f.image != "" && f.repo != "":
		return createPlan{}, newValidationError("--image and --repo are mutually exclusive (existing-image vs git-build paths); use --file for a full app.yaml spec instead")
	case f.image != "":
		return planExistingImage(f)
	case f.repo != "" || detected.RepoURL != "":
		return planGitBuildFlags(f, detected)
	default:
		return createPlan{}, newValidationError("specify one of: --file <app.yaml>, --image <ref> (existing-image path), or --repo <url> (git-build path)")
	}
}

func planExistingImage(f createFlags) (createPlan, error) {
	var missing []string
	if f.name == "" {
		missing = append(missing, "--name")
	}
	if f.port <= 0 {
		missing = append(missing, "--port")
	}
	if len(missing) > 0 {
		return createPlan{}, newValidationError("missing required flag(s) for the existing-image path: %s", strings.Join(missing, ", "))
	}
	return createPlan{
		CreateBody: appResource{Name: f.name, Image: f.image, Port: f.port},
	}, nil
}

func planGitBuildFlags(f createFlags, detected detectedGit) (createPlan, error) {
	repo := f.repo
	if repo == "" {
		repo = detected.RepoURL
	}
	ref := resolveRef(f.ref, detected.Ref)

	var missing []string
	if f.name == "" {
		missing = append(missing, "--name")
	}
	if f.port <= 0 {
		missing = append(missing, "--port")
	}
	if repo == "" {
		missing = append(missing, "--repo (no local git remote \"origin\" found to auto-detect)")
	}
	if f.imageRepo == "" {
		missing = append(missing, "--image-repo")
	}
	if len(missing) > 0 {
		return createPlan{}, newValidationError("missing required flag(s) for the git-build path: %s", strings.Join(missing, ", "))
	}

	return createPlan{
		CreateBody: appResource{Name: f.name, Image: pendingImageTag(f.imageRepo), Port: f.port},
		Build: &buildTriggerRequest{
			RepoURL:   repo,
			Ref:       ref,
			ImageRepo: f.imageRepo,
			Build:     buildTriggerRequestBuild{Path: f.dockerfile},
		},
	}, nil
}

func planFromFile(f createFlags, fileSpec *spec.Spec, detected detectedGit) (createPlan, error) {
	if fileSpec == nil {
		return createPlan{}, newValidationError("internal error: --file given but no parsed spec provided")
	}

	key, err := selectService(fileSpec, f.service)
	if err != nil {
		return createPlan{}, err
	}
	svc := fileSpec.Services[key]

	if svc.Build.Type != spec.BuildDockerfile {
		return createPlan{}, newValidationError("service %q has build.type %q; apps create only supports %q today", key, svc.Build.Type, spec.BuildDockerfile)
	}

	if secretKeys := secretEnvKeys(svc.Env); len(secretKeys) > 0 {
		return createPlan{}, newValidationError(
			"service %q declares secret env var(s) %s; apps create does not yet set secrets, create the app without them and set each one via PUT /api/v1/apps/{name}/secrets/{key} afterward",
			key, strings.Join(secretKeys, ", "))
	}

	name := f.name
	if name == "" {
		name = key
	}

	port := f.port
	if port <= 0 {
		port = svc.Port
	}
	if port <= 0 {
		return createPlan{}, newValidationError("service %q has no port set in %s and --port was not given", key, f.file)
	}

	if f.imageRepo == "" {
		return createPlan{}, newValidationError("--image-repo is required: app.yaml has no field for it")
	}

	repo := f.repo
	if repo == "" {
		repo = detected.RepoURL
	}
	if repo == "" {
		return createPlan{}, newValidationError("--repo is required (no local git remote \"origin\" found to auto-detect)")
	}
	ref := resolveRef(f.ref, detected.Ref)

	dockerfile := f.dockerfile
	if dockerfile == "" {
		dockerfile = svc.Build.Path
	}

	resources, err := toServiceResources(svc.Resources)
	if err != nil {
		return createPlan{}, newValidationError("service %q resources: %v", key, err)
	}
	health, err := toServiceHealth(svc.Health)
	if err != nil {
		return createPlan{}, newValidationError("service %q health: %v", key, err)
	}

	return createPlan{
		CreateBody: appResource{
			Name:      name,
			Image:     pendingImageTag(f.imageRepo),
			Port:      port,
			Domains:   svc.Domains,
			Env:       literalEnv(svc.Env),
			Resources: resources,
			Health:    health,
		},
		Build: &buildTriggerRequest{
			RepoURL:   repo,
			Ref:       ref,
			ImageRepo: f.imageRepo,
			Build:     buildTriggerRequestBuild{Path: dockerfile},
		},
	}, nil
}

// resolveRef applies the --ref > detected-branch > "main" precedence
// the CLI's flags surface documents.
func resolveRef(flagRef, detectedRef string) string {
	if flagRef != "" {
		return flagRef
	}
	if detectedRef != "" {
		return detectedRef
	}
	return "main"
}

func selectService(fileSpec *spec.Spec, serviceFlag string) (string, error) {
	if serviceFlag != "" {
		if _, ok := fileSpec.Services[serviceFlag]; !ok {
			return "", newValidationError("--service %q not found; app spec declares: %s", serviceFlag, strings.Join(serviceNames(fileSpec), ", "))
		}
		return serviceFlag, nil
	}
	if len(fileSpec.Services) == 1 {
		for k := range fileSpec.Services {
			return k, nil
		}
	}
	return "", newValidationError("app spec declares %d services; specify --service to choose one: %s", len(fileSpec.Services), strings.Join(serviceNames(fileSpec), ", "))
}

func serviceNames(fileSpec *spec.Spec) []string {
	names := make([]string, 0, len(fileSpec.Services))
	for k := range fileSpec.Services {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func secretEnvKeys(env map[string]spec.EnvVar) []string {
	var keys []string
	for k, v := range env {
		if v.Secret {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func literalEnv(env map[string]spec.EnvVar) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v.Value
	}
	return out
}

func toServiceResources(r *spec.Resources) (*serviceResources, error) {
	if r == nil {
		return nil, nil
	}
	var out serviceResources
	if r.Memory != "" {
		bytes, err := parseMemoryBytes(r.Memory)
		if err != nil {
			return nil, err
		}
		out.MemoryBytes = bytes
	}
	if r.CPU != 0 {
		out.NanoCPUs = int64(r.CPU * 1e9)
	}
	return &out, nil
}

func toServiceHealth(h *spec.Health) (*serviceHealth, error) {
	if h == nil {
		return nil, nil
	}
	var out serviceHealth
	if h.Readiness != nil {
		p, err := toServiceProbe(*h.Readiness)
		if err != nil {
			return nil, fmt.Errorf("readiness: %w", err)
		}
		out.Readiness = &p
	}
	if h.Liveness != nil {
		p, err := toServiceProbe(*h.Liveness)
		if err != nil {
			return nil, fmt.Errorf("liveness: %w", err)
		}
		out.Liveness = &p
	}
	return &out, nil
}

func toServiceProbe(p spec.Probe) (serviceProbe, error) {
	interval, err := parseDurationOrZero(p.Interval)
	if err != nil {
		return serviceProbe{}, fmt.Errorf("interval: %w", err)
	}
	timeout, err := parseDurationOrZero(p.Timeout)
	if err != nil {
		return serviceProbe{}, fmt.Errorf("timeout: %w", err)
	}
	return serviceProbe{
		Path:     p.Path,
		Interval: interval.Nanoseconds(),
		Timeout:  timeout.Nanoseconds(),
		Failures: p.Failures,
	}, nil
}

func parseDurationOrZero(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	return d, nil
}

// parseMemoryBytes parses app.yaml's "512Mi"/"1Gi" shape into bytes.
// Deliberately reimplemented here rather than imported from
// internal/deploy: that package pulls in the BuildKit client and its own
// heavy transitive dependency tree, which this CLI binary (a thin HTTP
// client, nothing more) must not link against.
func parseMemoryBytes(s string) (int64, error) {
	digits := 0
	for digits < len(s) && s[digits] >= '0' && s[digits] <= '9' {
		digits++
	}
	if digits == 0 || digits == len(s) {
		return 0, fmt.Errorf("invalid memory value %q, want a number followed by Mi or Gi", s)
	}
	var num int64
	if _, err := fmt.Sscanf(s[:digits], "%d", &num); err != nil {
		return 0, fmt.Errorf("invalid memory value %q: %w", s, err)
	}
	switch unit := s[digits:]; unit {
	case "Mi":
		return num * 1024 * 1024, nil
	case "Gi":
		return num * 1024 * 1024 * 1024, nil
	default:
		return 0, fmt.Errorf("invalid memory unit %q in %q, want Mi or Gi", unit, s)
	}
}

// runAppsCreate is the I/O shell around planFromFlags: parse flags, read
// --file and detect local git if needed, execute the plan against a
// real Client, and print the result. Returns the process exit code.
func runAppsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var tokenFlag, apiURLFlag string
	f, err := parseCreateFlags(prog, args, stderr, &tokenFlag, &apiURLFlag)
	if err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	var fileSpec *spec.Spec
	if f.file != "" {
		data, readErr := os.ReadFile(f.file) //nolint:gosec // operator-supplied CLI flag, the same "caller-controlled config path" pattern the control plane's own spec-file reads already use
		if readErr != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("read %s: %w", f.file, readErr))
		}
		parsed, parseErr := spec.Parse(data)
		if parseErr != nil {
			return reportError(stdout, stderr, f.jsonOut, newValidationError("parse %s: %v", f.file, parseErr))
		}
		fileSpec = parsed
	}

	var detected detectedGit
	if f.repo == "" {
		detected = detectLocalGit()
	}

	plan, err := planFromFlags(f, fileSpec, detected)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}

	token := resolveToken(tokenFlag, lookupEnv, prog)
	apiURL := resolveAPIURL(apiURLFlag, lookupEnv, prog)
	client := NewClient(apiURL, token)
	ctx := context.Background()

	created, err := client.CreateApp(ctx, plan.CreateBody)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("create app %q: %w", plan.CreateBody.Name, err))
	}

	if plan.Build != nil {
		if !f.jsonOut {
			_, _ = fmt.Fprintf(stderr, "app %q created, building from %s (ref %s)...\n", created.Name, plan.Build.RepoURL, plan.Build.Ref)
		}
		if _, buildErr := client.TriggerBuild(ctx, created.Name, *plan.Build); buildErr != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("app %q was created but the build failed: %w", created.Name, buildErr))
		}
	}

	final, err := client.GetApp(ctx, created.Name)
	if err != nil {
		// The app (and, if applicable, its build) already succeeded by
		// this point; a failure here only means the CLI can't show the
		// final state, not that anything upstream failed, so this
		// deliberately doesn't call reportError's "the whole command
		// failed" path. Still exits non-zero: an agent driving this CLI
		// should not treat "couldn't confirm the result" as success.
		_, _ = fmt.Fprintf(stderr, "app %q created, but re-fetching its final state failed: %v\n", created.Name, err)
		return exitNetwork
	}

	if f.jsonOut {
		if err := writeJSONValue(stdout, final); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stderr, "app %q created\n", final.Name)
	printAppHuman(stdout, final)
	return exitOK
}

// parseCreateFlags parses "apps create"'s flags, including the common
// --token/--api-url flags (bound into tokenFlag/apiURLFlag) so -h/--help
// output lists every flag this command accepts in one place.
// flag.ContinueOnError (not flag.ExitOnError) throughout this CLI: a
// library-triggered os.Exit would make every command untestable, so
// every command function returns an int exit code instead and only
// main() itself ever calls os.Exit.
func parseCreateFlags(prog string, args []string, errOut io.Writer, tokenFlag, apiURLFlag *string) (createFlags, error) {
	fs := flag.NewFlagSet(prog+" apps create", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() { _, _ = fmt.Fprint(errOut, appsCreateUsage(prog)) }

	var f createFlags
	fs.StringVar(&f.name, "name", "", "app name (required unless --file is given and app.yaml has exactly one service)")
	fs.StringVar(&f.image, "image", "", "existing image to deploy, e.g. registry.example.com/org/app:tag (existing-image path)")
	fs.IntVar(&f.port, "port", 0, "container port the app listens on")
	fs.StringVar(&f.repo, "repo", "", "git repository URL to build from (git-build path); auto-detected from the current directory's git remote \"origin\" when omitted")
	fs.StringVar(&f.ref, "ref", "", "git ref (branch) to build from (git-build path); defaults to the current directory's branch, then \"main\"")
	fs.StringVar(&f.dockerfile, "dockerfile", "", "Dockerfile path relative to the repo root (git-build path); defaults to \"Dockerfile\" at the repo root")
	fs.StringVar(&f.imageRepo, "image-repo", "", "image name without a tag, e.g. registry.example.com/org/app (git-build path)")
	fs.StringVar(&f.file, "file", "", "path to an app.yaml (or equivalent) spec file; an alternative to the flag-only paths above")
	fs.StringVar(&f.service, "service", "", "which service in --file's services: map to create, required when it declares more than one")
	fs.BoolVar(&f.yes, "yes", false, "accept defaults without prompting (reserved: this command never prompts today, so this is currently a no-op accepted for forward compatibility and script portability)")
	fs.BoolVar(&f.yes, "y", false, "shorthand for --yes")
	fs.BoolVar(&f.jsonOut, "json", false, "print the created app as JSON to stdout and nothing else")
	fs.StringVar(tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")

	if err := fs.Parse(args); err != nil {
		return createFlags{}, err
	}
	return f, nil
}

// reportError prints err on stderr always, and additionally as a JSON
// error object on stdout when jsonOut is set (the --json contract:
// "prints the created app resource (or an error object) as JSON to
// stdout, no other output in that mode"). Returns the exit code err
// classifies to. Takes stdout explicitly rather than reaching for
// os.Stdout, the same "every command function takes its writers as
// arguments" rule this whole CLI follows so it's testable without
// touching the real process stdout/stderr.
func reportError(stdout, stderr io.Writer, jsonOut bool, err error) int {
	_, _ = fmt.Fprintln(stderr, err)
	if jsonOut {
		if jerr := writeJSONError(stdout, err); jerr != nil {
			_, _ = fmt.Fprintln(stderr, jerr)
		}
	}
	return exitCodeForError(err)
}

func appsCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps create --name NAME --image IMAGE --port PORT [flags]
  %[1]s apps create --name NAME --port PORT --repo URL --image-repo REPO [flags]
  %[1]s apps create --file app.yaml [flags]

Creates an app, either by registering an existing image, or by
triggering a real build from a git repository. Flags present skip any
corresponding prompt; flags absent (outside --json/--yes) currently fail
with a clear error rather than prompting, so this command behaves the
same whether a human or a script is driving it.

Existing-image path:
  --name string        app name (required)
  --image string        image reference to deploy, e.g. registry.example.com/org/app:tag (required)
  --port int             container port (required)

Git-build path (flags):
  --name string          app name (required)
  --port int              container port (required)
  --repo string           git repository URL (required unless auto-detected from ./.git's "origin" remote)
  --ref string            git ref/branch (default: current branch, then "main")
  --image-repo string     image name without a tag (required)
  --dockerfile string   Dockerfile path relative to the repo root (default: "Dockerfile")

Manifest path:
  --file string           path to an app.yaml (or equivalent) spec file
  --service string      which service to create, if the file declares more than one
  --name, --port, --repo, --ref, --dockerfile, --image-repo above all override
    the file's own values or supply what it cannot express (repo location, image name)

Common flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print the result as JSON to stdout, nothing else
  --yes, -y                accept defaults without prompting (reserved, currently a no-op)
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
