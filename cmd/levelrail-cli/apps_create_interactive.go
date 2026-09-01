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
}

// toSpec builds the single-service app spec wizardAnswers describes,
// validating it the same way a hand-written app.yaml would be
// (spec.Spec.Validate, the semantic-rules layer Parse itself runs after
// schema validation) before any caller writes it out or sends it
// anywhere.
func (a wizardAnswers) toSpec() (*spec.Spec, error) {
	svc := spec.Service{Port: a.port}
	switch a.sourceKind {
	case wizardSourceImage:
		svc.Build = spec.Build{Type: spec.BuildImage, Image: a.image}
	default:
		svc.Build = spec.Build{Type: spec.BuildDockerfile}
	}
	if a.domain != "" {
		svc.Domains = []string{a.domain}
	}
	if a.healthPath != "" {
		probe := spec.Probe{Path: a.healthPath}
		svc.Health = &spec.Health{Readiness: &probe, Liveness: &probe}
	}
	if a.memory != "" || a.cpu > 0 {
		svc.Resources = &spec.Resources{Memory: a.memory, CPU: a.cpu}
	}

	s := &spec.Spec{Version: 1, Services: map[string]spec.Service{a.serviceName: svc}}
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("wizard produced an invalid app spec: %w", err)
	}
	return s, nil
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
// limits, then where to send the result. detected is the current
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

	port, err := p.readInt("Container port (the port your app listens on inside the container): ")
	if err != nil {
		return wizardAnswers{}, err
	}
	a.port = port

	domain, err := p.readOptional("Domain to route to this app (optional, press enter to skip): ", "")
	if err != nil {
		return wizardAnswers{}, err
	}
	a.domain = domain

	health, err := p.readLine("Health check path (default: /healthz, type 'skip' for none): ")
	if err != nil {
		return wizardAnswers{}, err
	}
	switch strings.ToLower(health) {
	case "":
		a.healthPath = "/healthz"
	case "skip", "none", "no":
		a.healthPath = ""
	default:
		a.healthPath = health
	}

	for {
		mem, err := p.readLine("Memory limit, e.g. 512Mi or 1Gi (optional, press enter for no limit): ")
		if err != nil {
			return wizardAnswers{}, err
		}
		if mem == "" {
			break
		}
		if _, parseErr := parseMemoryBytes(mem); parseErr != nil {
			_, _ = fmt.Fprintln(p.stderr, parseErr)
			continue
		}
		a.memory = mem
		break
	}

	for {
		cpuStr, err := p.readLine("CPU limit in cores, e.g. 0.5 or 1 (optional, press enter for no limit): ")
		if err != nil {
			return wizardAnswers{}, err
		}
		if cpuStr == "" {
			break
		}
		cpu, convErr := strconv.ParseFloat(cpuStr, 64)
		if convErr != nil || cpu <= 0 {
			_, _ = fmt.Fprintln(p.stderr, "enter a positive number, e.g. 0.5")
			continue
		}
		a.cpu = cpu
		break
	}

	mode, err := p.readChoice("Write app.yaml to the current directory, or create the app directly via the API? [file/api] (default: file): ", "file", "file", "api")
	if err != nil {
		return wizardAnswers{}, err
	}
	if mode == "api" {
		a.outputMode = wizardOutputAPI
		if a.sourceKind == wizardSourceGit {
			imageRepo, err := p.readRequired("Image repository to push the build to, e.g. registry.example.com/org/app: ")
			if err != nil {
				return wizardAnswers{}, err
			}
			a.imageRepo = imageRepo
		}
	} else {
		a.outputMode = wizardOutputFile
	}

	return a, nil
}

// wizardAppYAMLPath is where the wizard's file output mode writes,
// matching the primary candidate spec.DiscoverPath checks first and
// every example in docs/getting-started.md.
const wizardAppYAMLPath = "app.yaml"

// runAppsCreateWizard is "apps create --interactive"'s I/O shell: run
// the question loop, then either write app.yaml or create the app via
// the API, per the wizard's own last answer.
func runAppsCreateWizard(stdin io.Reader, stdout, stderr io.Writer, tokenFlag, apiURLFlag, profileFlag string, jsonOut bool, lookupEnv func(string) (string, bool), prog string) int {
	p := newWizardPrompter(stdin, stderr)
	answers, err := runInteractiveWizard(p, detectLocalGit())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	if answers.outputMode == wizardOutputFile {
		return runWizardWriteFile(answers, stdout, stderr, jsonOut)
	}
	return runWizardCreateViaAPI(answers, stdout, stderr, tokenFlag, apiURLFlag, profileFlag, jsonOut, lookupEnv, prog)
}

func runWizardWriteFile(a wizardAnswers, stdout, stderr io.Writer, jsonOut bool) int {
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

	if jsonOut {
		if err := writeJSONValue(stdout, map[string]string{"file": wizardAppYAMLPath}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stderr, "wrote %s\n", wizardAppYAMLPath)
	_, _ = stdout.Write(data)
	return exitOK
}

func runWizardCreateViaAPI(a wizardAnswers, stdout, stderr io.Writer, tokenFlag, apiURLFlag, profileFlag string, jsonOut bool, lookupEnv func(string) (string, bool), prog string) int {
	plan, err := a.toCreatePlan()
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	profile := resolveProfile(profileFlag, lookupEnv)
	token := resolveToken(tokenFlag, lookupEnv, prog, profile)
	apiURL := resolveAPIURL(apiURLFlag, lookupEnv, prog, profile)
	client := NewClient(apiURL, token)
	ctx := context.Background()

	created, err := client.CreateApp(ctx, plan.CreateBody)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create app %q: %w", plan.CreateBody.Name, err))
	}

	if buildErr := triggerCreatePlanBuild(ctx, client, created, plan, stderr, jsonOut); buildErr != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("app %q was created but the build failed: %w", created.Name, buildErr))
	}

	return fetchAndPrintCreatedApp(ctx, client, created.Name, stdout, stderr, jsonOut)
}
