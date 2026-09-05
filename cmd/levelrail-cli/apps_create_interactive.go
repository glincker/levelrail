package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/spec"
	"gopkg.in/yaml.v3"
)

// wizardSourceKind is which of the two ways to get a running container
// the wizard's "source" step selected.
type wizardSourceKind int

const (
	wizardSourceGit wizardSourceKind = iota
	wizardSourceImage
)

// wizardOutputMode is the wizard's last step: what to do with the
// answers gathered so far.
type wizardOutputMode int

const (
	wizardOutputFile wizardOutputMode = iota
	wizardOutputAPI
)

// wizardAnswers is everything runInteractiveWizard collects, converted
// to a real app spec by toSpec. Kept separate from the prompt loop
// itself so the spec-building and plan-building logic below is a pure
// function over plain data, table-driven testable with no io.Reader in
// the loop at all, the same separation createFlags/planFromFlags use.
type wizardAnswers struct {
	serviceName string
	sourceKind  wizardSourceKind
	image       string
	repoURL     string
	ref         string
	port        int
	domain      string
	healthPath  string
	memory      string
	cpu         float64
	outputMode  wizardOutputMode
	// imageRepo is only collected (and only meaningful) for the git
	// source combined with the API output mode: app.yaml has no field
	// for it (spec.Build has no ImageRepo), but a real build trigger
	// request does, see buildTriggerRequest.
	imageRepo string
	// extra holds every service beyond the first one, collected by the
	// wizard's "add another service?" loop. Empty (the common case)
	// means a single-service app, keeping every existing single-service
	// code path (toCreatePlan, the plain CreateApp+build trigger) byte
	// for byte unchanged.
	extra []wizardService
	// appName and imageRepoBase are only collected, and only meaningful,
	// when extra is non-empty and outputMode is API: a multi-service
	// deploy goes through POST /api/v1/apps/{name}/deploy-spec instead
	// of CreateApp, which needs an app-level name distinct from any one
	// service's name plus its own optional image tag prefix.
	appName       string
	imageRepoBase string
}

// wizardService is one additional service collected by the wizard's
// "add another service?" loop: the same shape as wizardAnswers' own
// primary-service fields, minus the ones only meaningful for the
// single-service CreateApp path (repoURL/ref/imageRepo), plus
// baseDirectory for a monorepo service built from the shared repo a
// multi-service deploy-spec request already carries at the top level.
type wizardService struct {
	serviceName   string
	sourceKind    wizardSourceKind
	image         string
	baseDirectory string
	port          int
	domain        string
	healthPath    string
	memory        string
	cpu           float64
}

// toSpecService builds the spec.Service s describes, the same
// construction wizardAnswers.toSpec used to do inline for its own
// single primary service.
func (s wizardService) toSpecService() spec.Service {
	svc := spec.Service{Port: s.port}
	switch s.sourceKind {
	case wizardSourceImage:
		svc.Build = spec.Build{Type: spec.BuildImage, Image: s.image}
	default:
		svc.Build = spec.Build{Type: spec.BuildDockerfile, BaseDirectory: s.baseDirectory}
	}
	if s.domain != "" {
		svc.Domains = []string{s.domain}
	}
	if s.healthPath != "" {
		probe := spec.Probe{Path: s.healthPath}
		svc.Health = &spec.Health{Readiness: &probe, Liveness: &probe}
	}
	if s.memory != "" || s.cpu > 0 {
		svc.Resources = &spec.Resources{Memory: s.memory, CPU: s.cpu}
	}
	return svc
}

// toSpec builds the single-service app spec wizardAnswers describes,
// validating it the same way a hand-written app.yaml would be
// (spec.Spec.Validate, the semantic-rules layer Parse itself runs after
// schema validation) before any caller writes it out or sends it
// anywhere.
func (a wizardAnswers) toSpec() (*spec.Spec, error) {
	primary := wizardService{
		serviceName: a.serviceName, sourceKind: a.sourceKind, image: a.image,
		port: a.port, domain: a.domain, healthPath: a.healthPath, memory: a.memory, cpu: a.cpu,
	}
	services := map[string]spec.Service{a.serviceName: primary.toSpecService()}
	for _, e := range a.extra {
		services[e.serviceName] = e.toSpecService()
	}

	s := &spec.Spec{Version: 1, Services: services}
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("wizard produced an invalid app spec: %w", err)
	}
	return s, nil
}

// toDeploySpecRequest builds the POST /api/v1/apps/{name}/deploy-spec
// body for a multi-service wizard run (len(a.extra) > 0): the same
// services map toSpec builds, converted service by service with
// apps_deploy_spec.go's own toDeploySpecService so a wizard-driven
// multi-service deploy validates (e.g. rejects secret env vars) exactly
// like "apps deploy-spec --file" does.
func (a wizardAnswers) toDeploySpecRequest() (deploySpecRequest, error) {
	s, err := a.toSpec()
	if err != nil {
		return deploySpecRequest{}, err
	}
	req := deploySpecRequest{
		RepoURL: a.repoURL, Ref: a.ref, ImageRepoBase: a.imageRepoBase,
		Services: make(map[string]deploySpecService, len(s.Services)),
	}
	for key, svc := range s.Services {
		converted, convErr := toDeploySpecService(svc)
		if convErr != nil {
			return deploySpecRequest{}, fmt.Errorf("service %q: %w", key, convErr)
		}
		req.Services[key] = converted
	}
	return req, nil
}

// appYAML renders a.toSpec() as app.yaml bytes, round-tripped through
// spec.Parse (schema validation plus the semantic checks Validate
// already ran) before being handed to a caller: the same
// build-then-validate shape coolify_yaml.go's buildAppYAML uses for the
// migration command's generated app.yaml.
func (a wizardAnswers) appYAML() ([]byte, error) {
	s, err := a.toSpec()
	if err != nil {
		return nil, err
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("render app.yaml: %w", err)
	}
	if _, err := spec.Parse(data); err != nil {
		return nil, fmt.Errorf("generated app.yaml failed validation: %w", err)
	}
	return data, nil
}

// toCreatePlan turns a into the same createPlan the --file path builds,
// by handing a synthetic single-service spec.Spec to planFromFile: this
// is the reuse point for toServiceResources/toServiceHealth and every
// other rule already encoded there (secret env rejection, missing-port
// messages, and so on), rather than a second copy of that logic here.
func (a wizardAnswers) toCreatePlan() (createPlan, error) {
	s, err := a.toSpec()
	if err != nil {
		return createPlan{}, err
	}
	f := createFlags{name: a.serviceName}
	if a.sourceKind == wizardSourceGit {
		f.repo = a.repoURL
		f.ref = a.ref
		f.imageRepo = a.imageRepo
	}
	return planFromFile(f, s, detectedGit{})
}

// wizardPrompter drives the interactive question loop over a plain
// bufio.Scanner: no interactive-prompt library is a dependency of this
// module today (see go.mod), and a step-by-step wizard is simple enough
// that a stdin-based scan loop covers it without adding one just for
// this. All output goes to stderr, this CLI's diagnostics stream, so a
// script that pipes stdout for --json is never polluted by a prompt.
type wizardPrompter struct {
	scanner *bufio.Scanner
	stderr  io.Writer
}

func newWizardPrompter(stdin io.Reader, stderr io.Writer) *wizardPrompter {
	return &wizardPrompter{scanner: bufio.NewScanner(stdin), stderr: stderr}
}

// readLine writes prompt and reads one line. An EOF on stdin (a script
// that ran --interactive with no terminal attached, or simply ran out
// of scripted answers) is reported as a validationError, the same
// "refuse, don't hang, don't guess" outcome authprompt.go's own
// promptUsername gives for the identical case.
func (p *wizardPrompter) readLine(prompt string) (string, error) {
	_, _ = fmt.Fprint(p.stderr, prompt)
	if !p.scanner.Scan() {
		if err := p.scanner.Err(); err != nil {
			return "", newValidationError("read input: %v", err)
		}
		return "", newValidationError("read input: unexpected end of input")
	}
	return strings.TrimSpace(p.scanner.Text()), nil
}

func (p *wizardPrompter) readRequired(prompt string) (string, error) {
	for {
		v, err := p.readLine(prompt)
		if err != nil {
			return "", err
		}
		if v != "" {
			return v, nil
		}
		_, _ = fmt.Fprintln(p.stderr, "this is required, please enter a value")
	}
}

func (p *wizardPrompter) readOptional(prompt, def string) (string, error) {
	v, err := p.readLine(prompt)
	if err != nil {
		return "", err
	}
	if v == "" {
		return def, nil
	}
	return v, nil
}

func (p *wizardPrompter) readInt(prompt string) (int, error) {
	for {
		v, err := p.readLine(prompt)
		if err != nil {
			return 0, err
		}
		n, convErr := strconv.Atoi(v)
		if convErr != nil || n <= 0 {
			_, _ = fmt.Fprintln(p.stderr, "enter a positive whole number")
			continue
		}
		return n, nil
	}
}

// readOptionalInt is readInt with a default for a blank answer instead of
// requiring a positive value: blank means def (used by the databases
// wizard's backup retain/retain-days prompts, where 0 means "no limit",
// the same convention backups schedule set's own --retain/--retain-days
// flags already use).
func (p *wizardPrompter) readOptionalInt(prompt string, def int) (int, error) {
	for {
		v, err := p.readLine(prompt)
		if err != nil {
			return 0, err
		}
		if v == "" {
			return def, nil
		}
		n, convErr := strconv.Atoi(v)
		if convErr != nil || n < 0 {
			_, _ = fmt.Fprintln(p.stderr, "enter a non-negative whole number, or press enter to skip")
			continue
		}
		return n, nil
	}
}

// readChoice loops until v (or def, when v is blank) case-insensitively
// matches one of options, returning the matched option in its
// canonical casing.
func (p *wizardPrompter) readChoice(prompt, def string, options ...string) (string, error) {
	for {
		v, err := p.readLine(prompt)
		if err != nil {
			return "", err
		}
		if v == "" {
			v = def
		}
		for _, opt := range options {
			if strings.EqualFold(v, opt) {
				return opt, nil
			}
		}
		_, _ = fmt.Fprintf(p.stderr, "please enter one of: %s\n", strings.Join(options, ", "))
	}
}

// runInteractiveWizard is the wizard's question loop: app name, source
// (git repo or Docker image), port, domain, health check path, resource
// limits, an optional "add another service?" loop for a multi-service
// app, then where to send the result. detected is the current
// directory's best-effort local git info (same detectLocalGit the
// flag-driven path already uses), offered as a default so a caller
// standing inside a checkout doesn't have to retype its own remote URL.
func runInteractiveWizard(p *wizardPrompter, detected detectedGit) (wizardAnswers, error) {
	var a wizardAnswers

	rawName, err := p.readRequired("App name: ")
	if err != nil {
		return wizardAnswers{}, err
	}
	a.serviceName = sanitizeServiceName(rawName)
	if a.serviceName != strings.ToLower(strings.TrimSpace(rawName)) {
		_, _ = fmt.Fprintf(p.stderr, "using %q (sanitized to lowercase alphanumeric and hyphens, starting with a letter)\n", a.serviceName)
	}

	source, err := p.readChoice("Deploy from a git repository or an existing Docker image? [git/image] (default: git): ", "git", "git", "image")
	if err != nil {
		return wizardAnswers{}, err
	}
	if source == "image" {
		a.sourceKind = wizardSourceImage
		image, err := p.readRequired("Docker image reference (e.g. registry.example.com/org/app:tag): ")
		if err != nil {
			return wizardAnswers{}, err
		}
		a.image = image
	} else {
		a.sourceKind = wizardSourceGit
		repoPrompt := "Git repository URL: "
		if detected.RepoURL != "" {
			repoPrompt = fmt.Sprintf("Git repository URL (default: %s): ", detected.RepoURL)
		}
		repo, err := p.readOptional(repoPrompt, detected.RepoURL)
		if err != nil {
			return wizardAnswers{}, err
		}
		if repo == "" {
			return wizardAnswers{}, newValidationError("a git repository URL is required")
		}
		a.repoURL = repo
		a.ref = resolveRef("", detected.Ref)
	}

	common, err := readCommonServiceAnswers(p, "Container port (the port your app listens on inside the container): ", "Domain to route to this app (optional, press enter to skip): ")
	if err != nil {
		return wizardAnswers{}, err
	}
	a.port, a.domain, a.healthPath, a.memory, a.cpu = common.port, common.domain, common.healthPath, common.memory, common.cpu

	for {
		more, err := p.readChoice("Add another service to this app? [y/N] (default: N): ", "n", "y", "n")
		if err != nil {
			return wizardAnswers{}, err
		}
		if more == "n" {
			break
		}
		extraSvc, err := readExtraService(p, len(a.extra)+2)
		if err != nil {
			return wizardAnswers{}, err
		}
		a.extra = append(a.extra, extraSvc)
	}

	mode, err := p.readChoice("Write app.yaml to the current directory, or create the app directly via the API? [file/api] (default: file): ", "file", "file", "api")
	if err != nil {
		return wizardAnswers{}, err
	}
	if mode != "api" {
		a.outputMode = wizardOutputFile
		return a, nil
	}
	a.outputMode = wizardOutputAPI

	if len(a.extra) == 0 {
		if a.sourceKind == wizardSourceGit {
			imageRepo, err := p.readRequired("Image repository to push the build to, e.g. registry.example.com/org/app: ")
			if err != nil {
				return wizardAnswers{}, err
			}
			a.imageRepo = imageRepo
		}
		return a, nil
	}

	// A multi-service create goes through POST .../deploy-spec instead
	// of CreateApp: it needs an app-level name distinct from any one
	// service's name, and (see handleDeploySpec, internal/api/
	// apps_multi.go) a repo_url/ref pair unconditionally, even when
	// every service happens to be build.type: image, which a git-source
	// primary service already collected above.
	appName, err := p.readRequired("App name (the app's identifier on the control plane, distinct from any one service's name): ")
	if err != nil {
		return wizardAnswers{}, err
	}
	a.appName = appName
	if a.sourceKind != wizardSourceGit {
		repo, err := p.readRequired("Git repository URL to build every service from (required for a multi-service deploy): ")
		if err != nil {
			return wizardAnswers{}, err
		}
		a.repoURL = repo
		ref, err := p.readOptional("Branch, tag, or commit to build (default: main): ", "main")
		if err != nil {
			return wizardAnswers{}, err
		}
		a.ref = ref
	}
	imageRepoBase, err := p.readOptional("Image repository prefix for built services (optional, defaults to the app name): ", "")
	if err != nil {
		return wizardAnswers{}, err
	}
	a.imageRepoBase = imageRepoBase

	return a, nil
}

// wizardCommonServiceAnswers is the prompt block every service, primary
// or extra, shares: port, domain, health check, and resource limits.
type wizardCommonServiceAnswers struct {
	port       int
	domain     string
	healthPath string
	memory     string
	cpu        float64
}

func readCommonServiceAnswers(p *wizardPrompter, portPrompt, domainPrompt string) (wizardCommonServiceAnswers, error) {
	var c wizardCommonServiceAnswers

	port, err := p.readInt(portPrompt)
	if err != nil {
		return wizardCommonServiceAnswers{}, err
	}
	c.port = port

	domain, err := p.readOptional(domainPrompt, "")
	if err != nil {
		return wizardCommonServiceAnswers{}, err
	}
	c.domain = domain

	health, err := p.readLine("Health check path (default: /healthz, type 'skip' for none): ")
	if err != nil {
		return wizardCommonServiceAnswers{}, err
	}
	switch strings.ToLower(health) {
	case "":
		c.healthPath = "/healthz"
	case "skip", "none", "no":
		c.healthPath = ""
	default:
		c.healthPath = health
	}

	for {
		mem, err := p.readLine("Memory limit, e.g. 512Mi or 1Gi (optional, press enter for no limit): ")
		if err != nil {
			return wizardCommonServiceAnswers{}, err
		}
		if mem == "" {
			break
		}
		if _, parseErr := parseMemoryBytes(mem); parseErr != nil {
			_, _ = fmt.Fprintln(p.stderr, parseErr)
			continue
		}
		c.memory = mem
		break
	}

	for {
		cpuStr, err := p.readLine("CPU limit in cores, e.g. 0.5 or 1 (optional, press enter for no limit): ")
		if err != nil {
			return wizardCommonServiceAnswers{}, err
		}
		if cpuStr == "" {
			break
		}
		cpu, convErr := strconv.ParseFloat(cpuStr, 64)
		if convErr != nil || cpu <= 0 {
			_, _ = fmt.Fprintln(p.stderr, "enter a positive number, e.g. 0.5")
			continue
		}
		c.cpu = cpu
		break
	}

	return c, nil
}

// readExtraService prompts for one service beyond the first, ordinal
// being its 1-based position among all services (2, 3, ...) for the
// prompt text. Unlike the primary service, its git-vs-image choice
// never asks for a repository: a multi-service deploy always builds
// every git-sourced service from the one shared repo/ref collected once
// for the whole app (see the appName/imageRepoBase block above), so this
// only needs the subdirectory within it.
func readExtraService(p *wizardPrompter, ordinal int) (wizardService, error) {
	var s wizardService

	rawName, err := p.readRequired(fmt.Sprintf("Service #%d name: ", ordinal))
	if err != nil {
		return wizardService{}, err
	}
	s.serviceName = sanitizeServiceName(rawName)
	if s.serviceName != strings.ToLower(strings.TrimSpace(rawName)) {
		_, _ = fmt.Fprintf(p.stderr, "using %q (sanitized to lowercase alphanumeric and hyphens, starting with a letter)\n", s.serviceName)
	}

	source, err := p.readChoice("Deploy from the shared git repository or a Docker image? [git/image] (default: git): ", "git", "git", "image")
	if err != nil {
		return wizardService{}, err
	}
	if source == "image" {
		s.sourceKind = wizardSourceImage
		image, err := p.readRequired("Docker image reference (e.g. registry.example.com/org/app:tag): ")
		if err != nil {
			return wizardService{}, err
		}
		s.image = image
	} else {
		s.sourceKind = wizardSourceGit
		baseDir, err := p.readOptional("Subdirectory this service builds from within the repo, e.g. apps/api (optional, blank for the repo root): ", "")
		if err != nil {
			return wizardService{}, err
		}
		s.baseDirectory = baseDir
	}

	common, err := readCommonServiceAnswers(p, "Container port (the port this service listens on inside the container): ", "Domain to route to this service (optional, press enter to skip): ")
	if err != nil {
		return wizardService{}, err
	}
	s.port, s.domain, s.healthPath, s.memory, s.cpu = common.port, common.domain, common.healthPath, common.memory, common.cpu

	return s, nil
}

// wizardAppYAMLPath is where the wizard's file output mode writes,
// matching the primary candidate spec.DiscoverPath checks first and
// every example in docs/getting-started.md.
const wizardAppYAMLPath = "app.yaml"

// runAppsCreateWizard is "apps create --interactive"'s I/O shell: run
// the question loop, then either write app.yaml or create the app via
// the API, per the wizard's own last answer.
func runAppsCreateWizard(stdin io.Reader, stdout, stderr io.Writer, cf credentialFlags, of outputFlags, lookupEnv func(string) (string, bool), prog string) int {
	jsonOut := of.Format == outputJSON
	p := newWizardPrompter(stdin, stderr)
	answers, err := runInteractiveWizard(p, detectLocalGit())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	if answers.outputMode == wizardOutputFile {
		return runWizardWriteFile(answers, stdout, stderr, of)
	}
	return runWizardCreateViaAPI(answers, stdout, stderr, cf, of, lookupEnv, prog)
}

func runWizardWriteFile(a wizardAnswers, stdout, stderr io.Writer, of outputFlags) int {
	jsonOut := of.Format == outputJSON
	if _, statErr := os.Stat(wizardAppYAMLPath); statErr == nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s already exists in the current directory; remove or rename it first", wizardAppYAMLPath))
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("check %s: %w", wizardAppYAMLPath, statErr))
	}

	data, err := a.appYAML()
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if err := os.WriteFile(wizardAppYAMLPath, data, 0o644); err != nil { //nolint:gosec // app.yaml is meant to be readable by whoever deploys with it, the same permissions git itself would give a checked-in file
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("write %s: %w", wizardAppYAMLPath, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]string{"file": wizardAppYAMLPath}, func() {
		_, _ = fmt.Fprintf(stderr, "wrote %s\n", wizardAppYAMLPath)
		_, _ = stdout.Write(data)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runWizardCreateViaAPI(a wizardAnswers, stdout, stderr io.Writer, cf credentialFlags, of outputFlags, lookupEnv func(string) (string, bool), prog string) int {
	if len(a.extra) > 0 {
		return runWizardCreateMultiServiceViaAPI(a, stdout, stderr, cf, of, lookupEnv, prog)
	}

	jsonOut := of.Format == outputJSON
	plan, err := a.toCreatePlan()
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	client := apiClientFromCredentialFlags(prog, cf, lookupEnv)
	ctx := context.Background()

	created, err := client.CreateApp(ctx, plan.CreateBody)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create app %q: %w", plan.CreateBody.Name, err))
	}

	if buildErr := triggerCreatePlanBuild(ctx, client, created, plan, stderr, jsonOut); buildErr != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("app %q was created but the build failed: %w", created.Name, buildErr))
	}

	return fetchAndPrintCreatedApp(ctx, client, created.Name, stdout, stderr, of)
}

// runWizardCreateMultiServiceViaAPI is runWizardCreateViaAPI's path for
// a multi-service wizard run: POST .../deploy-spec instead of CreateApp,
// the same call "apps deploy-spec" itself makes, printed the same way
// (printDeploySpecResultHuman) so the two surfaces read identically.
func runWizardCreateMultiServiceViaAPI(a wizardAnswers, stdout, stderr io.Writer, cf credentialFlags, of outputFlags, lookupEnv func(string) (string, bool), prog string) int {
	jsonOut := of.Format == outputJSON
	req, err := a.toDeploySpecRequest()
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	client := apiClientFromCredentialFlags(prog, cf, lookupEnv)
	result, err := client.DeploySpec(context.Background(), a.appName, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("deploy spec to app %q: %w", a.appName, err))
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
