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
	name  string
	image string
	port  int
	// hostPort backs --host-port: 0 means unset (auto-assign), matching
	// port's own zero-means-unset convention, since a real host port is
	// always positive. See toHostPort below for how this becomes
	// appResource.HostPort's *int.
	hostPort      int
	repo          string
	ref           string
	dockerfile    string
	buildType     string
	baseDirectory string
	// buildArgs backs --build-arg (repeatable KEY=VALUE), only meaningful
	// alongside --build-type dockerfile. See stringMapFlag (flagutil.go).
	buildArgs  map[string]string
	imageRepo  string
	file       string
	service    string
	yes        bool
	jsonOut    bool
	outputFlag string
	queryFlag  string
	// interactive backs --interactive: runs a step-by-step wizard
	// instead of requiring every flag up front. See
	// apps_create_interactive.go's runAppsCreateWizard.
	interactive bool

	// attachDatabase, attachDatabaseEnvVar, attachDatabaseField back
	// --attach-database and its two optional refinements: a post-create
	// call to PUT /api/v1/apps/{name}/database (apiclient's
	// SetAppDatabaseAttachment), the CLI's own reach into the real
	// app-to-database attachment internal/api/apps_database.go exposes,
	// matching this repo's own "UI, CLI, and API together" rule. Not part
	// of createPlan: unlike CreateBody/Build, this is a separate HTTP
	// call runAppsCreate makes only once the app itself already exists.
	attachDatabase       string
	attachDatabaseEnvVar string
	attachDatabaseField  string
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
		CreateBody: appResource{Name: f.name, Image: f.image, Port: f.port, HostPort: toHostPort(f.hostPort)},
	}, nil
}

func planGitBuildFlags(f createFlags, detected detectedGit) (createPlan, error) {
	repo := f.repo
	if repo == "" {
		repo = detected.RepoURL
	}
	ref := resolveRef(f.ref, detected.Ref)

	buildType := f.buildType
	if buildType == "" {
		buildType = spec.BuildDockerfile
	}
	if buildType != spec.BuildDockerfile && buildType != spec.BuildRailpack {
		return createPlan{}, newValidationError("--build-type %q not supported by the git-build path; use %q or %q", buildType, spec.BuildDockerfile, spec.BuildRailpack)
	}
	if buildType != spec.BuildDockerfile && len(f.buildArgs) > 0 {
		return createPlan{}, newValidationError("--build-arg is only meaningful with --build-type %q", spec.BuildDockerfile)
	}

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

	buildInput := buildTriggerRequestBuild{Type: buildType, BaseDirectory: f.baseDirectory}
	if buildType == spec.BuildDockerfile {
		buildInput.Path = f.dockerfile
		buildInput.Args = f.buildArgs
	}

	return createPlan{
		CreateBody: appResource{Name: f.name, Image: pendingImageTag(f.imageRepo), Port: f.port, HostPort: toHostPort(f.hostPort)},
		Build: &buildTriggerRequest{
			RepoURL:   repo,
			Ref:       ref,
			ImageRepo: f.imageRepo,
			Build:     buildInput,
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

	switch svc.Build.Type {
	case spec.BuildDockerfile, spec.BuildRailpack:
		return planFromFileBuild(f, key, svc, detected, svc.Build.Type)
	case spec.BuildImage:
		return planFromFileImage(f, key, svc)
	default:
		return createPlan{}, newValidationError("service %q has build.type %q; apps create only supports %q, %q, and %q today", key, svc.Build.Type, spec.BuildDockerfile, spec.BuildRailpack, spec.BuildImage)
	}
}

func planFromFileBuild(f createFlags, key string, svc spec.Service, detected detectedGit, buildType string) (createPlan, error) {
	if buildType != spec.BuildDockerfile && len(f.buildArgs) > 0 {
		return createPlan{}, newValidationError("--build-arg is only meaningful for a dockerfile build (service %q has build.type %q)", key, buildType)
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
	hostPort := f.hostPort
	if hostPort <= 0 {
		hostPort = svc.HostPort
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

	baseDirectory := f.baseDirectory
	if baseDirectory == "" {
		baseDirectory = svc.Build.BaseDirectory
	}
	buildInput := buildTriggerRequestBuild{Type: buildType, BaseDirectory: baseDirectory}
	if buildType == spec.BuildDockerfile {
		dockerfile := f.dockerfile
		if dockerfile == "" {
			dockerfile = svc.Build.Path
		}
		buildInput.Path = dockerfile

		args := svc.Build.Args
		if len(f.buildArgs) > 0 {
			args = f.buildArgs
		}
		buildInput.Args = args
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
			HostPort:  toHostPort(hostPort),
			Domains:   svc.Domains,
			Env:       literalEnv(svc.Env),
			Resources: resources,
			Health:    health,
		},
		Build: &buildTriggerRequest{
			RepoURL:   repo,
			Ref:       ref,
			ImageRepo: f.imageRepo,
			Build:     buildInput,
		},
	}, nil
}

// planFromFileImage handles a service whose build.type is image: a
// prebuilt registry reference, so the app is created directly with that
// image, no build trigger and no repo/ref/image-repo needed at all.
func planFromFileImage(f createFlags, key string, svc spec.Service) (createPlan, error) {
	if svc.Build.Image == "" {
		return createPlan{}, newValidationError("service %q has build.type %q but no build.image set", key, spec.BuildImage)
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
	hostPort := f.hostPort
	if hostPort <= 0 {
		hostPort = svc.HostPort
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
			Image:     svc.Build.Image,
			Port:      port,
			HostPort:  toHostPort(hostPort),
			Domains:   svc.Domains,
			Env:       literalEnv(svc.Env),
			Resources: resources,
			Health:    health,
		},
	}, nil
}

// toHostPort turns createFlags.hostPort's 0-means-unset int into
// appResource.HostPort's nil-means-unset *int.
func toHostPort(hostPort int) *int {
	if hostPort == 0 {
		return nil
	}
	return &hostPort
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
	if r.SwapMemory != "" {
		bytes, err := parseMemoryBytes(r.SwapMemory)
		if err != nil {
			return nil, err
		}
		// Docker's MemorySwap is memory+swap combined, not swap on top of
		// memory, so a value below the memory limit is nonsensical: it
		// would otherwise surface as a raw Docker API rejection at
		// container-create time instead of here. Mirrors
		// internal/deploy/translate.go's toServiceResources.
		if bytes < out.MemoryBytes {
			return nil, fmt.Errorf("resources.swapMemory (%s) must be at least resources.memory (%s): it is memory+swap combined, not swap alone", r.SwapMemory, r.Memory)
		}
		out.SwapMemoryBytes = bytes
	}
	out.CPUSetCPUs = r.CPUSet
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
// stdin is only ever read from when --interactive is given.
func runAppsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	var tokenFlag, apiURLFlag, profileFlag string
	f, err := parseCreateFlags(prog, args, stderr, &tokenFlag, &apiURLFlag, &profileFlag)
	if err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	format, ferr := resolveOutputFormat(f.jsonOut, f.outputFlag)
	if ferr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, ferr)
		return exitValidation
	}
	of := outputFlags{format, f.queryFlag}

	if f.interactive {
		if f.name != "" || f.image != "" || f.repo != "" || f.file != "" {
			return reportError(stdout, stderr, f.jsonOut, newValidationError("--interactive runs its own step-by-step prompts and cannot be combined with --name, --image, --repo, or --file"))
		}
		return runAppsCreateWizard(stdin, stdout, stderr, credentialFlags{Token: tokenFlag, APIURL: apiURLFlag, Profile: profileFlag}, of, lookupEnv, prog)
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

	profile := resolveProfile(profileFlag, lookupEnv)
	token := resolveToken(tokenFlag, lookupEnv, prog, profile)
	apiURL := resolveAPIURL(apiURLFlag, lookupEnv, prog, profile)
	client := NewClient(apiURL, token)
	ctx := context.Background()

	created, err := client.CreateApp(ctx, plan.CreateBody)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("create app %q: %w", plan.CreateBody.Name, err))
	}

	if buildErr := triggerCreatePlanBuild(ctx, client, created, plan, stderr, f.jsonOut); buildErr != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("app %q was created but the build failed: %w", created.Name, buildErr))
	}

	if f.attachDatabase != "" {
		if !f.jsonOut {
			_, _ = fmt.Fprintf(stderr, "attaching database %q...\n", f.attachDatabase)
		}
		attachReq := setAppDatabaseRequest{DatabaseName: f.attachDatabase, EnvVar: f.attachDatabaseEnvVar, Field: f.attachDatabaseField}
		if _, attachErr := client.SetAppDatabaseAttachment(ctx, created.Name, attachReq); attachErr != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("app %q was created but attaching database %q failed: %w", created.Name, f.attachDatabase, attachErr))
		}
	}

	return fetchAndPrintCreatedApp(ctx, client, created.Name, stdout, stderr, of)
}

// triggerCreatePlanBuild triggers plan's build (a no-op returning nil
// when plan.Build is nil, the existing-image path), shared between the
// flag-driven and wizard-driven "apps create" I/O shells.
func triggerCreatePlanBuild(ctx context.Context, client *Client, created appResource, plan createPlan, stderr io.Writer, jsonOut bool) error {
	if plan.Build == nil {
		return nil
	}
	if !jsonOut {
		_, _ = fmt.Fprintf(stderr, "app %q created, building from %s (ref %s)...\n", created.Name, plan.Build.RepoURL, plan.Build.Ref)
	}
	_, err := client.TriggerBuild(ctx, created.Name, *plan.Build)
	return err
}

// fetchAndPrintCreatedApp re-fetches name's now-final state and prints
// it, the shared tail of every "apps create" path once the app (and, if
// applicable, its build) already succeeded.
func fetchAndPrintCreatedApp(ctx context.Context, client *Client, name string, stdout, stderr io.Writer, of outputFlags) int {
	final, err := client.GetApp(ctx, name)
	if err != nil {
		// The app (and, if applicable, its build) already succeeded by
		// this point; a failure here only means the CLI can't show the
		// final state, not that anything upstream failed, so this
		// deliberately doesn't call reportError's "the whole command
		// failed" path. Still exits non-zero: an agent driving this CLI
		// should not treat "couldn't confirm the result" as success.
		_, _ = fmt.Fprintf(stderr, "app %q created, but re-fetching its final state failed: %v\n", name, err)
		return exitNetwork
	}

	if err := renderResult(stdout, of.Format, of.Query, final, func() {
		_, _ = fmt.Fprintf(stderr, "app %q created\n", final.Name)
		printAppHuman(stdout, final)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// parseCreateFlags parses "apps create"'s flags, including the common
// --token/--api-url flags (bound into tokenFlag/apiURLFlag) so -h/--help
// output lists every flag this command accepts in one place.
// flag.ContinueOnError (not flag.ExitOnError) throughout this CLI: a
// library-triggered os.Exit would make every command untestable, so
// every command function returns an int exit code instead and only
// main() itself ever calls os.Exit.
func parseCreateFlags(prog string, args []string, errOut io.Writer, tokenFlag, apiURLFlag, profileFlag *string) (createFlags, error) {
	fs := flag.NewFlagSet(prog+" apps create", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() { _, _ = fmt.Fprint(errOut, appsCreateUsage(prog)) }

	var f createFlags
	fs.StringVar(&f.name, "name", "", "app name (required unless --file is given and app.yaml has exactly one service)")
	fs.StringVar(&f.image, "image", "", "existing image to deploy, e.g. registry.example.com/org/app:tag (existing-image path)")
	fs.IntVar(&f.port, "port", 0, "container port the app listens on")
	fs.IntVar(&f.hostPort, "host-port", 0, "host port to pin the container port to (default: auto-assigned by Docker)")
	fs.StringVar(&f.repo, "repo", "", "git repository URL to build from (git-build path); auto-detected from the current directory's git remote \"origin\" when omitted")
	fs.StringVar(&f.ref, "ref", "", "git ref (branch) to build from (git-build path); defaults to the current directory's branch, then \"main\"")
	fs.StringVar(&f.dockerfile, "dockerfile", "", "Dockerfile path relative to the repo root (git-build path, --build-type dockerfile only); defaults to \"Dockerfile\" at the repo root")
	fs.StringVar(&f.buildType, "build-type", "", "build type for the git-build path: \"dockerfile\" (default) or \"railpack\" (auto-detected build, no Dockerfile needed)")
	fs.StringVar(&f.baseDirectory, "base-directory", "", "subdirectory of the repo to build from, for a monorepo (git-build path); defaults to the repo root")
	f.buildArgs = make(map[string]string)
	fs.Var(stringMapFlag(f.buildArgs), "build-arg", "Dockerfile build arg as KEY=VALUE, repeatable (git-build path, --build-type dockerfile only)")
	fs.StringVar(&f.imageRepo, "image-repo", "", "image name without a tag, e.g. registry.example.com/org/app (git-build path)")
	fs.StringVar(&f.file, "file", "", "path to an app.yaml (or equivalent) spec file; an alternative to the flag-only paths above")
	fs.StringVar(&f.service, "service", "", "which service in --file's services: map to create, required when it declares more than one")
	fs.StringVar(&f.attachDatabase, "attach-database", "", "name of an existing managed database to attach after create (injects a connection env var, see --attach-database-env-var/--attach-database-field)")
	fs.StringVar(&f.attachDatabaseEnvVar, "attach-database-env-var", "", "env var name the attached database's value is injected as (default: DATABASE_URL); only meaningful with --attach-database")
	fs.StringVar(&f.attachDatabaseField, "attach-database-field", "", "which field to inject: url, host, port, username, password, or database (default: url); only meaningful with --attach-database")
	fs.BoolVar(&f.yes, "yes", false, "accept defaults without prompting (reserved: no-op outside --interactive, accepted for forward compatibility and script portability)")
	fs.BoolVar(&f.yes, "y", false, "shorthand for --yes")
	fs.BoolVar(&f.interactive, "interactive", false, "run a step-by-step wizard instead of specifying flags: prompts for name, source, port, domain, health check, and resource limits, then writes app.yaml or calls the API")
	fs.BoolVar(&f.interactive, "i", false, "shorthand for --interactive")
	fs.BoolVar(&f.jsonOut, "json", false, "print the created app as JSON to stdout and nothing else")
	fs.StringVar(tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.StringVar(profileFlag, "profile", "", "named credentials profile to read (overrides "+envProfile+", default \""+defaultProfile+"\")")
	outputFlagP, queryFlagP := bindOutputQueryFlags(fs)

	if err := fs.Parse(args); err != nil {
		return createFlags{}, err
	}
	f.outputFlag, f.queryFlag = *outputFlagP, *queryFlagP
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
  %[1]s apps create --interactive

Creates an app, either by registering an existing image, or by
triggering a real build from a git repository. Flags present skip any
corresponding prompt; flags absent (outside --json/--yes/--interactive)
fail with a clear error rather than prompting, so this command behaves
the same whether a human or a script is driving it, unless --interactive
is given.

Interactive path:
  --interactive, -i   run a step-by-step wizard: prompts for name,
                       source (git repo or Docker image), port, domain,
                       health check path, and resource limits, then asks
                       whether to write app.yaml or create the app via
                       the API. Cannot be combined with --name, --image,
                       --repo, or --file.

Existing-image path:
  --name string        app name (required)
  --image string        image reference to deploy, e.g. registry.example.com/org/app:tag (required)
  --port int             container port (required)
  --host-port int      host port to pin the container port to (default: auto-assigned by Docker)

Git-build path (flags):
  --name string          app name (required)
  --port int              container port (required)
  --host-port int      host port to pin the container port to (default: auto-assigned by Docker)
  --repo string           git repository URL (required unless auto-detected from ./.git's "origin" remote)
  --ref string            git ref/branch (default: current branch, then "main")
  --image-repo string     image name without a tag (required)
  --build-type string  "dockerfile" (default) or "railpack" (auto-detected build, no Dockerfile needed)
  --dockerfile string   Dockerfile path relative to the repo root, --build-type dockerfile only (default: "Dockerfile")
  --base-directory string  subdirectory of the repo to build from, for a monorepo (default: repo root)
  --build-arg KEY=VALUE   Dockerfile build arg, repeatable, --build-type dockerfile only

Manifest path:
  --file string           path to an app.yaml (or equivalent) spec file
  --service string      which service to create, if the file declares more than one
  --name, --port, --host-port, --repo, --ref, --dockerfile, --base-directory, --build-arg, --image-repo above
    all override the file's own values or supply what it cannot express (repo location, image name)
  build.type: dockerfile or railpack builds from git (repo/ref/image-repo required, as above);
    build.type: image creates the app directly with build.image, no build triggered
  build.args from app.yaml's own build.args flow through automatically for a dockerfile build;
    --build-arg overrides them entirely rather than merging

Database attachment (any path above):
  --attach-database string           name of an existing managed database to attach after create
  --attach-database-env-var string   env var name for the injected value (default: DATABASE_URL)
  --attach-database-field string     url, host, port, username, password, or database (default: url)

Common flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --yes, -y                accept defaults without prompting (reserved, currently a no-op)
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
