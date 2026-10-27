package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// createFlags is the raw flag input to "apps create", before any mode inference or defaulting.
type createFlags struct {
	name  string
	image string
	port  int
	// hostPort backs --host-port; 0 means unset (auto-assign).
	hostPort int
	// bindAddress backs --bind-address: "" means unset (falls through to
	// the server's own default, "private"), matching appResource.BindAddress's
	// own empty-means-unset-not-invalid convention.
	bindAddress   string
	repo          string
	ref           string
	dockerfile    string
	buildType     string
	framework     string
	baseDirectory string
	// buildArgs backs --build-arg (repeatable KEY=VALUE), only meaningful
	// alongside --build-type dockerfile. See stringMapFlag (flagutil.go).
	buildArgs  map[string]string
	imageRepo  string
	file       string
	service    string
	jsonOut    bool
	outputFlag string
	queryFlag  string
	// interactive backs --interactive: runs a step-by-step wizard
	// instead of requiring every flag up front. See
	// apps_create_interactive.go's runAppsCreateWizard.
	interactive bool

	// nodeIDSet tells an omitted --node-id (control plane auto-places) apart from an explicit empty one (pins to local).
	replicas  int
	strategy  string
	nodeID    string
	nodeIDSet bool

	// attachDatabase* back --attach-database, applied with a separate PUT once the app exists.
	attachDatabase       string
	attachDatabaseEnvVar string
	attachDatabaseField  string

	// secrets back --secret; applied to CreateBody after planning, whichever path built the plan.
	secrets map[string]string

	// vaultSecrets back --vault-secret; applied after planning, like secrets.
	vaultSecrets map[string]appVaultEnvRef

	// readiness, liveness and readyTimeout back the --readiness-* / --liveness-* / --ready-timeout health flags.
	readiness, liveness probeFlags
	readyTimeout        string
}

// createPlan is the validated request set runAppsCreate sends. Build is nil for an existing image; CreateBody and Build are zero for a static --file service.
type createPlan struct {
	CreateBody appResource
	Build      *buildTriggerRequest
	DeploySpec *deploySpecCreatePlan
	// Volumes are sent with a follow-up PUT after create, since POST /apps never accepts them.
	Volumes []spec.Volume
}

// deploySpecCreatePlan is a single-service POST /apps/{name}/deploy-spec request, used for static sites.
type deploySpecCreatePlan struct {
	AppName    string
	ServiceKey string
	Request    deploySpecRequest
}

// pendingImageTag is the placeholder image POST /apps requires for git builds; the first successful build overwrites it.
func pendingImageTag(imageRepo string) string {
	return imageRepo + spec.PendingImageTag
}

// planFromFlags picks the creation path (existing image, git build, or --file) and builds its requests. Pure, so it is table-tested.
// A local git remote also selects the git-build path when no --image or --file is given.
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
		CreateBody: appResource{Name: f.name, Image: f.image, Port: f.port, HostPort: toHostPort(f.hostPort), BindAddress: f.bindAddress},
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
		CreateBody: appResource{Name: f.name, Image: pendingImageTag(f.imageRepo), Port: f.port, HostPort: toHostPort(f.hostPort), BindAddress: f.bindAddress},
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
	if err := checkUnappliedSections(fileSpec, key, "levelrail", f.file); err != nil {
		return createPlan{}, err
	}
	svc := fileSpec.Services[key]
	if err := checkRequiredSecrets(f, key, svc); err != nil {
		return createPlan{}, err
	}

	switch svc.Build.Type {
	case spec.BuildDockerfile, spec.BuildRailpack:
		return planFromFileBuild(f, key, svc, detected, svc.Build.Type)
	case spec.BuildImage:
		return planFromFileImage(f, key, svc)
	case spec.BuildStatic:
		return planFromFileStatic(f, key, svc, detected)
	default:
		return createPlan{}, newValidationError("service %q has build.type %q; apps create only supports %q, %q, %q, and %q today", key, svc.Build.Type, spec.BuildDockerfile, spec.BuildRailpack, spec.BuildImage, spec.BuildStatic)
	}
}

func planFromFileBuild(f createFlags, key string, svc spec.Service, detected detectedGit, buildType string) (createPlan, error) {
	if buildType != spec.BuildDockerfile && len(f.buildArgs) > 0 {
		return createPlan{}, newValidationError("--build-arg is only meaningful for a dockerfile build (service %q has build.type %q)", key, buildType)
	}
	secretEnv := secretEnvKeys(svc.Env)

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
	bindAddress := f.bindAddress
	if bindAddress == "" {
		bindAddress = svc.BindAddress
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
			Name:        name,
			Image:       pendingImageTag(f.imageRepo),
			Port:        port,
			HostPort:    toHostPort(hostPort),
			BindAddress: bindAddress,
			Domains:     svc.Domains,
			Env:         literalEnv(svc.Env),
			SecretEnv:   secretEnv,
			Resources:   resources,
			Health:      health,
			Command:     svc.Command,
			Replicas:    svc.Replicas,
			Strategy:    svc.Strategy,
		},
		Build: &buildTriggerRequest{
			RepoURL:   repo,
			Ref:       ref,
			ImageRepo: f.imageRepo,
			Build:     buildInput,
		},
		Volumes: svc.Volumes,
	}, nil
}

// planFromFileImage handles a service whose build.type is image: a
// prebuilt registry reference, so the app is created directly with that
// image, no build trigger and no repo/ref/image-repo needed at all.
func planFromFileImage(f createFlags, key string, svc spec.Service) (createPlan, error) {
	if svc.Build.Image == "" {
		return createPlan{}, newValidationError("service %q has build.type %q but no build.image set", key, spec.BuildImage)
	}
	secretEnv := secretEnvKeys(svc.Env)

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
	bindAddress := f.bindAddress
	if bindAddress == "" {
		bindAddress = svc.BindAddress
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
			Name:        name,
			Image:       svc.Build.Image,
			Port:        port,
			HostPort:    toHostPort(hostPort),
			BindAddress: bindAddress,
			Domains:     svc.Domains,
			Env:         literalEnv(svc.Env),
			SecretEnv:   secretEnv,
			Resources:   resources,
			Health:      health,
			Command:     svc.Command,
			Replicas:    svc.Replicas,
			Strategy:    svc.Strategy,
		},
		Volumes: svc.Volumes,
	}, nil
}

// planFromFileStatic handles build.type: static services, which cannot go through POST /apps (it requires a port).
// single_service_name names the site after the app itself.
func planFromFileStatic(f createFlags, key string, svc spec.Service, detected detectedGit) (createPlan, error) {
	var unsupported []string
	if f.port > 0 {
		unsupported = append(unsupported, "--port")
	}
	if f.hostPort > 0 {
		unsupported = append(unsupported, "--host-port")
	}
	if f.bindAddress != "" {
		unsupported = append(unsupported, "--bind-address")
	}
	if len(f.buildArgs) > 0 {
		unsupported = append(unsupported, "--build-arg")
	}
	if f.imageRepo != "" {
		unsupported = append(unsupported, "--image-repo")
	}
	if f.nodeIDSet {
		unsupported = append(unsupported, "--node-id")
	}
	if f.attachDatabase != "" {
		unsupported = append(unsupported, "--attach-database")
	}
	if len(f.vaultSecrets) > 0 {
		unsupported = append(unsupported, "--vault-secret")
	}
	if len(unsupported) > 0 {
		return createPlan{}, newValidationError("service %q has build.type %q, which has no running container: %s not supported", key, spec.BuildStatic, strings.Join(unsupported, ", "))
	}

	name := f.name
	if name == "" {
		name = key
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
	buildPath := f.dockerfile
	if buildPath == "" {
		buildPath = svc.Build.Path
	}

	svcSpec := toDeploySpecService(svc)
	svcSpec.Build.Type = spec.BuildStatic
	svcSpec.Build.Path = buildPath
	svcSpec.Build.BaseDirectory = baseDirectory
	services := map[string]deploySpecService{key: svcSpec}
	applyDeploySpecSecrets(services, f.secrets)

	return createPlan{
		DeploySpec: &deploySpecCreatePlan{
			AppName:    name,
			ServiceKey: key,
			Request: deploySpecRequest{
				RepoURL:           repo,
				Ref:               ref,
				Services:          services,
				SingleServiceName: name,
			},
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

// literalEnv skips secret and from: entries, which have no literal value this endpoint can send.
func literalEnv(env map[string]spec.EnvVar) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		if v.Secret || v.From != "" {
			continue
		}
		out[k] = v.Value
	}
	if len(out) == 0 {
		return nil
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
		// Docker's MemorySwap is memory plus swap combined, so a value below the memory limit is invalid.
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
	readyTimeout, err := parseDurationOrZero(h.ReadyTimeout)
	if err != nil {
		return nil, fmt.Errorf("readyTimeout: %w", err)
	}
	out.ReadyTimeout = readyTimeout.Nanoseconds()
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
		Path:            p.Path,
		Scheme:          p.Scheme,
		Host:            p.Host,
		TLSSkipVerify:   p.TLSSkipVerify,
		FollowRedirects: p.FollowRedirects,
		ExpectedStatus:  string(p.ExpectedStatus),
		Exec:            []string(p.Exec),
		Interval:        interval.Nanoseconds(),
		Timeout:         timeout.Nanoseconds(),
		Failures:        p.Failures,
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

// parseMemoryBytes parses sizes like "512Mi". Reimplemented, not imported from internal/deploy, to keep BuildKit out of this binary.
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

// runAppsCreate is the I/O shell around planFromFlags. Returns the exit code. stdin is read only with --interactive.
func runAppsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	var tokenFlag, apiURLFlag, profileFlag string
	f, err := parseCreateFlags(prog, args, stderr, &tokenFlag, &apiURLFlag, &profileFlag)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
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

	profile := resolveProfile(profileFlag, lookupEnv)
	token := resolveToken(tokenFlag, lookupEnv, prog, profile)
	apiURL := resolveAPIURL(apiURLFlag, lookupEnv, prog, profile)
	client := NewClient(apiURL, token)
	ctx := context.Background()

	preset, err := applyFrameworkPreset(&f)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	applyGitBuildDefaults(&f, detected, ".", func(repo, ref string) (int, bool) {
		det, derr := client.DetectFramework(ctx, repo, ref)
		return det.Port, derr == nil && det.Detected && det.Port > 0
	})

	plan, err := planFromFlags(f, fileSpec, detected)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	if preset != nil {
		plan.Volumes = append(plan.Volumes, presetVolumes(preset)...)
		if !f.jsonOut {
			printPresetEnvHints(stderr, preset, f.secrets)
		}
	}
	// --node-id is applied after planning so every creation path honors it the same way.
	if f.nodeIDSet {
		plan.CreateBody.NodeID = f.nodeID
	}
	if f.replicas > 0 {
		plan.CreateBody.Replicas = f.replicas
	}
	if f.strategy != "" {
		plan.CreateBody.Strategy = f.strategy
	}
	if err := applyCreateHealth(f, &plan.CreateBody, plan.DeploySpec != nil); err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	if len(f.secrets) > 0 {
		plan.CreateBody.Secrets = f.secrets
	}
	if len(f.vaultSecrets) > 0 {
		plan.CreateBody.VaultEnv = f.vaultSecrets
	}

	if plan.DeploySpec != nil {
		return runAppsCreateStatic(ctx, client, plan.DeploySpec, of, stdout, stderr, f.jsonOut)
	}

	created, err := client.CreateApp(ctx, plan.CreateBody)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("create app %q: %w", plan.CreateBody.Name, err))
	}
	if created.AutoPlaced && !f.jsonOut {
		_, _ = fmt.Fprintf(stderr, "app %q auto-placed on node %q (simple spread scheduling)\n", created.Name, created.NodeID)
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

	if len(plan.Volumes) > 0 {
		if volErr := attachPlanVolumes(ctx, client, created.Name, plan.Volumes, stderr, f.jsonOut); volErr != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("app %q was created but attaching its volumes failed: %w", created.Name, volErr))
		}
	}

	return fetchAndPrintCreatedApp(ctx, client, created.Name, stdout, stderr, of)
}

// attachPlanVolumes wires app.yaml's volumes on with a follow-up PUT. Bind mounts (hostPath) are rejected, since that endpoint takes name and path only.
func attachPlanVolumes(ctx context.Context, client *Client, name string, volumes []spec.Volume, stderr io.Writer, jsonOut bool) error {
	resources := make([]appVolumeResource, 0, len(volumes))
	for _, v := range volumes {
		if v.HostPath != "" {
			return newValidationError("volume %q: hostPath bind mounts are not supported by \"apps create --file\", attach a named volume instead", v.Name)
		}
		resources = append(resources, appVolumeResource{Name: v.Name, ContainerPath: v.Path})
	}
	if !jsonOut {
		_, _ = fmt.Fprintf(stderr, "attaching %d volume(s)...\n", len(resources))
	}
	_, err := client.SetAppVolumes(ctx, name, setAppVolumesRequest{Volumes: resources})
	return err
}

// triggerCreatePlanBuild starts plan's build, a no-op for existing images. Framework detection is best effort and never blocks the create.
func triggerCreatePlanBuild(ctx context.Context, client *Client, created appResource, plan createPlan, stderr io.Writer, jsonOut bool) error {
	if plan.Build == nil {
		return nil
	}
	if plan.Build.DetectedFramework == "" {
		if det, err := client.DetectFramework(ctx, plan.Build.RepoURL, plan.Build.Ref); err == nil && det.Detected {
			plan.Build.DetectedFramework = det.FrameworkName
		}
	}
	if !jsonOut {
		_, _ = fmt.Fprintf(stderr, "app %q created, building from %s (ref %s)...\n", created.Name, plan.Build.RepoURL, plan.Build.Ref)
		_, _ = fmt.Fprintf(stderr, "follow it with \"apps deploys wait %s\"\n", created.Name)
	}
	_, err := client.TriggerBuild(ctx, created.Name, *plan.Build)
	return err
}

// runAppsCreateStatic executes a build.type: static plan via
// POST /apps/{name}/deploy-spec (see planFromFileStatic's own doc comment),
// printing the same deploy-spec result shape "apps deploy-spec" prints.
func runAppsCreateStatic(ctx context.Context, client *Client, plan *deploySpecCreatePlan, of outputFlags, stdout, stderr io.Writer, jsonOut bool) int {
	if !jsonOut {
		_, _ = fmt.Fprintf(stderr, "creating static site %q from %s (ref %s)...\n", plan.AppName, plan.Request.RepoURL, plan.Request.Ref)
	}
	result, err := client.DeploySpec(ctx, plan.AppName, plan.Request)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create static site %q: %w", plan.AppName, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, result, func() { printDeploySpecResultHuman(stdout, result) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if of.Format == outputTable && !result.AllSucceeded {
		return exitAPIError
	}
	return exitOK
}

// fetchAndPrintCreatedApp re-fetches name's now-final state and prints
// it, the shared tail of every "apps create" path once the app (and, if
// applicable, its build) already succeeded.
func fetchAndPrintCreatedApp(ctx context.Context, client *Client, name string, stdout, stderr io.Writer, of outputFlags) int {
	final, err := client.GetApp(ctx, name)
	if err != nil {
		// The app already exists here, so a failure only means the final state could not be shown. Still exits non-zero so scripts do not read it as success.
		_, _ = fmt.Fprintf(stderr, "app %q created, but re-fetching its final state failed: %v\n", name, err)
		return exitNetwork
	}

	if err := renderResult(stdout, of.Format, of.Query, final, func() {
		_, _ = fmt.Fprintf(stderr, "app %q created\n", final.Name)
		printAppHuman(stdout, final)
		printSelfSignedNote(ctx, client, stdout, final.FallbackURL)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// printSelfSignedNote says the automatic https URL is self-signed while
// HTTPS is not enabled. A failed status lookup prints nothing.
func printSelfSignedNote(ctx context.Context, client *Client, out io.Writer, autoURL string) {
	if !strings.HasPrefix(autoURL, "https://") {
		return
	}
	status, err := client.GetHTTPSStatus(ctx)
	if err != nil || status.State == "issued" {
		return
	}
	_, _ = fmt.Fprintln(out, "note:     the auto url uses a self-signed certificate until HTTPS is enabled; plain http gets a 308 redirect to it")
}

// ContinueOnError, not ExitOnError: a library os.Exit would make commands untestable, so only main() calls os.Exit.
func parseCreateFlags(prog string, args []string, errOut io.Writer, tokenFlag, apiURLFlag, profileFlag *string) (createFlags, error) {
	fs := flag.NewFlagSet(prog+" apps create", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() { _, _ = fmt.Fprint(errOut, appsCreateUsage(prog)) }

	var f createFlags
	fs.StringVar(&f.name, "name", "", "app name (default: the repo name on the git-build path, or the single service key with --file)")
	fs.StringVar(&f.image, "image", "", "existing image to deploy, e.g. registry.example.com/org/app:tag (existing-image path)")
	fs.IntVar(&f.port, "port", 0, "container port the app listens on (git-build default: detected from the checkout, the Dockerfile or the server's scan)")
	fs.IntVar(&f.hostPort, "host-port", 0, "host port to pin the container port to (default: auto-assigned by Docker)")
	fs.StringVar(&f.bindAddress, "bind-address", "", "network interface the port binds to: \"private\" (loopback only, the default), \"public\" (every interface), or a literal IP")
	fs.StringVar(&f.repo, "repo", "", "git repository URL to build from (git-build path); auto-detected from the current directory's git remote \"origin\" when omitted")
	fs.StringVar(&f.ref, "ref", "", "git ref (branch) to build from (git-build path); defaults to the current directory's branch, then \"main\"")
	fs.StringVar(&f.dockerfile, "dockerfile", "", "Dockerfile path relative to the repo root (git-build path, --build-type dockerfile only); defaults to \"Dockerfile\" at the repo root")
	fs.StringVar(&f.buildType, "build-type", "", "build type for the git-build path: \"dockerfile\" or \"railpack\" (auto-detected build, no Dockerfile needed); default: railpack when the local checkout has no Dockerfile, else dockerfile")
	fs.StringVar(&f.framework, "framework", "", "framework preset that fills the port, build type and volumes (git-build path); list them with \"build frameworks\"")
	fs.StringVar(&f.baseDirectory, "base-directory", "", "subdirectory of the repo to build from, for a monorepo (git-build path); defaults to the repo root")
	f.buildArgs = make(map[string]string)
	fs.Var(stringMapFlag(f.buildArgs), "build-arg", "Dockerfile build arg as KEY=VALUE, repeatable (git-build path, --build-type dockerfile only)")
	fs.StringVar(&f.imageRepo, "image-repo", "", "image name without a tag, e.g. registry.example.com/org/app (git-build path; default: the app name)")
	fs.StringVar(&f.file, "file", "", "path to an app.yaml (or equivalent) spec file; an alternative to the flag-only paths above")
	fs.StringVar(&f.service, "service", "", "which service in --file's services: map to create, required when it declares more than one")
	fs.IntVar(&f.replicas, "replicas", 0, "number of container replicas (default 1, or the spec's replicas with --file)")
	fs.StringVar(&f.strategy, "strategy", "", "deploy strategy: rolling, recreate, or blue-green (default blue-green, or the spec's strategy with --file)")
	fs.StringVar(&f.nodeID, "node-id", "", "node to place this app on (default: auto-placed on the least-loaded registered node, or the local node if only one exists)")
	fs.StringVar(&f.attachDatabase, "attach-database", "", "name of an existing managed database to attach after create (injects a connection env var, see --attach-database-env-var/--attach-database-field)")
	fs.StringVar(&f.attachDatabaseEnvVar, "attach-database-env-var", "", "env var name the attached database's value is injected as (default: DATABASE_URL); only meaningful with --attach-database")
	fs.StringVar(&f.attachDatabaseField, "attach-database-field", "", "which field to inject: url, host, port, username, password, or database (default: url); only meaningful with --attach-database")
	registerCreateHealthFlags(fs, &f)
	f.secrets = make(map[string]string)
	fs.Var(stringMapFlag(f.secrets), "secret", "secret env var value as KEY=VALUE, repeatable; stored via envelope-encrypted secret storage as part of this same create call (any path above), same value-on-the-command-line convention \"apps secrets set\" already uses")
	f.vaultSecrets = make(map[string]appVaultEnvRef)
	fs.Var(vaultEnvFlag(f.vaultSecrets), "vault-secret", "env var resolved live from an external Vault instance as KEY=path#field, repeatable; requires \"levelrail-cli vault set\" to have configured vault on this control plane, mutually exclusive per key with --secret")
	fs.BoolVar(&f.interactive, "interactive", false, "run a step-by-step wizard instead of specifying flags: prompts for name, source, port, domain, health check, resource limits, and optionally more services, then writes app.yaml or calls the API")
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
	fs.Visit(func(fl *flag.Flag) {
		if fl.Name == "node-id" {
			f.nodeIDSet = true
		}
	})
	return f, nil
}

// reportError writes err to stderr, and to stdout as a JSON error object when jsonOut is set. Writers are passed in so it is testable.
func reportError(stdout, stderr io.Writer, jsonOut bool, err error) int {
	_, _ = fmt.Fprintln(stderr, err)
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		if apiErr.Fix != "" {
			_, _ = fmt.Fprintf(stderr, "fix: %s\n", apiErr.Fix)
		} else if hint := publishedPortHint(apiErr.Code); hint != "" {
			_, _ = fmt.Fprintf(stderr, "hint: %s\n", hint)
		}
		if apiErr.RequestID != "" {
			_, _ = fmt.Fprintf(stderr, "request id: %s\n", apiErr.RequestID)
		}
	}
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
  %[1]s apps create --repo URL [--name NAME] [--port PORT] [--image-repo REPO] [flags]
  %[1]s apps create --file app.yaml [flags]
  %[1]s apps create --interactive

Creates an app, either by registering an existing image, or by
triggering a real build from a git repository. Flags present skip any
corresponding prompt; flags absent (outside --json/--interactive)
fail with a clear error rather than prompting, so this command behaves
the same whether a human or a script is driving it, unless --interactive
is given.

Interactive path:
  --interactive, -i   run a step-by-step wizard: prompts for name,
                       source (git repo or Docker image), port, domain,
                       health check path, and resource limits, then asks
                       whether to add another service (a multi-service
                       app), then whether to write app.yaml or create
                       the app via the API. Cannot be combined with
                       --name, --image, --repo, or --file.

Existing-image path:
  --name string        app name (required)
  --image string        image reference to deploy, e.g. registry.example.com/org/app:tag (required)
  --port int             container port (required)
  --host-port int      host port to pin the container port to (default: auto-assigned by Docker)
  --bind-address string  network interface the port binds to: "private" (default), "public", or a literal IP

Git-build path (flags):
  --name string          app name (default: the repo's name)
  --port int              container port (default: detected from the checkout, the Dockerfile or the server's scan)
  --host-port int      host port to pin the container port to (default: auto-assigned by Docker)
  --bind-address string  network interface the port binds to: "private" (default), "public", or a literal IP
  --repo string           git repository URL (required unless auto-detected from ./.git's "origin" remote)
  --ref string            git ref/branch (default: current branch, then "main")
  --image-repo string     image name without a tag (default: the app name)
  --build-type string  "dockerfile" or "railpack" (auto-detected build, no Dockerfile needed); default: railpack when the local checkout has no Dockerfile, else dockerfile
  --framework string      framework preset that fills the port, build type and volumes; list them with "build frameworks"
  --dockerfile string   Dockerfile path relative to the repo root, --build-type dockerfile only (default: "Dockerfile")
  --base-directory string  subdirectory of the repo to build from, for a monorepo (default: repo root)
  --build-arg KEY=VALUE   Dockerfile build arg, repeatable, --build-type dockerfile only

Manifest path:
  --file string           path to an app.yaml (or equivalent) spec file
  --service string      which service to create, if the file declares more than one
  --name, --port, --host-port, --bind-address, --repo, --ref, --dockerfile, --base-directory, --build-arg, --image-repo above
    all override the file's own values or supply what it cannot express (repo location, image name)
  build.type: dockerfile or railpack builds from git (repo/ref/image-repo required, as above);
    build.type: image creates the app directly with build.image, no build triggered;
    build.type: static deploys the built output directly (repo/ref required, no port/host-port/
      bind-address/build-arg/image-repo/node-id/attach-database/vault-secret, static sites have
      no running container)
  build.args from app.yaml's own build.args flow through automatically for a dockerfile build;
    --build-arg overrides them entirely rather than merging

Rollout (any path above):
  --replicas int          number of container replicas (default 1, or the spec's replicas with --file)
  --strategy string       deploy strategy: rolling, recreate or blue-green (default blue-green, or the spec's strategy with --file)

Placement (any path above):
  --node-id string        node to place this app on; omitted auto-places it on the
                                    least-loaded registered node (or the local node if only one exists)

Database attachment (any path above):
  --attach-database string           name of an existing managed database to attach after create
  --attach-database-env-var string   env var name for the injected value (default: DATABASE_URL)
  --attach-database-field string     url, host, port, username, password, or database (default: url)

Secrets (any path above):
  --secret KEY=VALUE      secret env var value, repeatable; stored via envelope-encrypted
                            secret storage as part of this same create call, so a { secret: true }
                            var declared in app.yaml (--file path) does not need a separate
                            PUT /api/v1/apps/{name}/secrets/{key} call afterward. A declared name
                            with no matching --secret still creates fine; set its value later the
                            same way.
  --vault-secret KEY=path#field   env var resolved live from an external Vault instance instead
                            of a value this platform stores, repeatable; requires
                            "%[1]s vault set" to have configured vault first. A
                            { vault: { path, key } } var declared in app.yaml (--file path) is
                            equivalent and needs no --vault-secret flag; this flag is for the
                            existing-image and git-build paths, which have no app.yaml to declare
                            it in.

Common flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// registerCreateHealthFlags adds the probe flags of "apps health set" under a readiness/liveness prefix, so both commands parse through probeFlags.toProbe.
func registerCreateHealthFlags(fs *flag.FlagSet, f *createFlags) {
	for _, p := range []struct {
		which string
		pf    *probeFlags
	}{{"readiness", &f.readiness}, {"liveness", &f.liveness}} {
		pf := p.pf
		pf.which = p.which
		fs.StringVar(&pf.path, p.which+"-path", "", "HTTP path for the "+p.which+" probe, e.g. /healthz")
		fs.StringVar(&pf.exec, p.which+"-exec", "", "command for the "+p.which+" probe, run through /bin/sh -c; exit 0 means healthy")
		fs.StringVar(&pf.preset, p.which+"-preset", "", "fill the "+p.which+" probe from a named preset ("+healthPresetNames+")")
		fs.StringVar(&pf.interval, p.which+"-interval", "", p.which+" probe interval, e.g. 5s")
		fs.StringVar(&pf.timeout, p.which+"-timeout", "", p.which+" probe per-attempt timeout, e.g. 2s")
		fs.StringVar(&pf.expectedStatus, p.which+"-expected-status", "", p.which+" probe accepted status codes, e.g. 200-399")
	}
	fs.IntVar(&f.liveness.failures, "liveness-failures", 0, "consecutive liveness failures before a restart")
	fs.StringVar(&f.readyTimeout, "ready-timeout", "", "how long a deploy waits for readiness, e.g. 90s")
}

func (p probeFlags) isSet() bool {
	return p.path != "" || p.exec != "" || p.preset != "" || p.interval != "" || p.timeout != "" || p.expectedStatus != "" || p.failures != 0
}

// applyCreateHealth overlays the health flags on the planned body; a flag probe replaces the same probe from --file.
func applyCreateHealth(f createFlags, body *appResource, static bool) error {
	if !f.readiness.isSet() && !f.liveness.isSet() && f.readyTimeout == "" {
		return nil
	}
	if static {
		return newValidationError("health flags are not supported for a static site, which has no running container")
	}
	health := serviceHealth{}
	if body.Health != nil {
		health = *body.Health
	}
	for _, pf := range []probeFlags{f.readiness, f.liveness} {
		if !pf.isSet() {
			continue
		}
		p, err := pf.toProbe()
		if err != nil {
			return newValidationError("%s probe: %v", pf.which, err)
		}
		if pf.which == "readiness" {
			health.Readiness = &p
		} else {
			health.Liveness = &p
		}
	}
	if f.readyTimeout != "" {
		d, err := parseDurationOrZero(f.readyTimeout)
		if err != nil {
			return newValidationError("--ready-timeout: %v", err)
		}
		health.ReadyTimeout = d.Nanoseconds()
	}
	body.Health = &health
	return nil
}
