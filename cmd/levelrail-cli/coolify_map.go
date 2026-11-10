package main

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// issueSeverity classifies a migrationIssue for report grouping.
type issueSeverity string

const (
	issueDropped  issueSeverity = "dropped"  // has no Levelrail equivalent, silently lost otherwise
	issueReview   issueSeverity = "review"   // migrated with a caveat the operator should check
	issueBlocking issueSeverity = "blocking" // the whole app could not be mapped at all
	issuePlanned  issueSeverity = "planned"  // carried as a follow-up step the import cannot apply directly
)

type migrationIssue struct {
	Field    string        `json:"field"`
	Severity issueSeverity `json:"severity"`
	Detail   string        `json:"detail"`
}

func newIssue(field string, severity issueSeverity, detail string) migrationIssue {
	return migrationIssue{Field: field, Severity: severity, Detail: detail}
}

// appendIssue is the shared "record a field-level migration issue" step
// used throughout both providers' mapping functions.
func appendIssue(issues []migrationIssue, field string, severity issueSeverity, detail string) []migrationIssue {
	return append(issues, newIssue(field, severity, detail))
}

// blockingIssue builds the issue returned alongside blocking=true by the
// build-type dispatch in both coolify_map.go and dokploy_map.go.
func blockingIssue(field, detail string) *migrationIssue {
	i := newIssue(field, issueBlocking, detail)
	return &i
}

// mappedApp is one migrated Coolify app: a generated service, or a blocking reason it cannot be migrated.
type mappedApp struct {
	SourceName  string           `json:"source_name"`
	ServiceName string           `json:"service_name,omitempty"`
	RepoURL     string           `json:"repo_url,omitempty"`
	Ref         string           `json:"ref,omitempty"`
	Service     mappedService    `json:"service"`
	Compose     *stagedCompose   `json:"compose,omitempty"`
	Files       []mappedFile     `json:"files,omitempty"`
	Issues      []migrationIssue `json:"issues,omitempty"`
	Blocking    bool             `json:"blocking"`

	// SecretsFile is set by writeMigrationFiles (file mode) when real env
	// values were fetched and written to a companion file.
	SecretsFile string `json:"secrets_file,omitempty"`

	// Applied/CreateError/SecretsSet/SecretsFailed are set by
	// applyMigration (--apply mode) only.
	Applied       bool     `json:"applied,omitempty"`
	CreateError   string   `json:"create_error,omitempty"`
	SecretsSet    []string `json:"secrets_set,omitempty"`
	SecretsFailed []string `json:"secrets_failed,omitempty"`

	// SourceServer is the source host the app ran on; Node is the target node --node-map put it on.
	SourceServer string `json:"source_server,omitempty"`
	Node         string `json:"node,omitempty"`
	// NetworkPeers are apps that shared a custom docker network with this one in the source.
	NetworkPeers []string `json:"network_peers,omitempty"`
	// Webhook is what --create-webhooks did, or would do, for a git-sourced app.
	Webhook      *webhookOutcome `json:"webhook,omitempty"`
	network      string
	nodeID       string
	gitSourceSet bool
}

type mappedService struct {
	BuildType string        `json:"build_type,omitempty"`
	BuildPath string        `json:"build_path,omitempty"`
	Domains   []string      `json:"domains,omitempty"`
	Port      int           `json:"port,omitempty"`
	Health    *mappedHealth `json:"health,omitempty"`
	Memory    string        `json:"memory,omitempty"`
	CPU       float64       `json:"cpu,omitempty"`
	EnvKeys   []string      `json:"env_keys,omitempty"`
	// Build settings carried from the source; commands holding an inline secret are never set here.
	BaseDirectory string       `json:"base_directory,omitempty"`
	BuildCommand  string       `json:"build_command,omitempty"`
	PublishDir    string       `json:"publish_dir,omitempty"`
	StartCommand  string       `json:"start_command,omitempty"`
	Hooks         *mappedHooks `json:"hooks,omitempty"`
	// BuildTimeKeys names variables the source exposed to the build; values live only in EnvLiteral.
	BuildTimeKeys []string `json:"build_time_keys,omitempty"`
	// Previews is set only by the Dokploy mapper, applied through the preview policy route.
	Previews *mappedPreviews `json:"previews,omitempty"`
	// Ingress holds validated auth, redirect and header settings from the source proxy; no credential.
	Ingress *platformimport.IngressPlan `json:"ingress,omitempty"`
	// EnvLiteral is set only when includeSecretValues was requested; it is never serialized or written to app.yaml.
	EnvLiteral map[string]string `json:"-"`
}

type mappedHooks struct {
	PreDeploy  string `json:"pre_deploy,omitempty"`
	PostDeploy string `json:"post_deploy,omitempty"`
}

type mappedPreviews struct {
	MaxPreviews int `json:"max_previews,omitempty"`
}

type mappedHealth struct {
	Path     string `json:"path"`
	Interval int    `json:"interval_seconds,omitempty"`
	Timeout  int    `json:"timeout_seconds,omitempty"`
	Failures int    `json:"failures,omitempty"`
}

var invalidNameChars = regexp.MustCompile(`[^a-z0-9-]+`)

// sanitizeServiceName maps free-text Coolify names onto the lowercase, hyphenated form app.yaml accepts.
func sanitizeServiceName(name string) string {
	s := invalidNameChars.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if s == "" {
		return "app"
	}
	if s[0] < 'a' || s[0] > 'z' {
		s = "app-" + s
	}
	return s
}

// mapServiceName sanitizes rawName and, when sanitization changed it,
// records a review issue naming providerName. Shared by mapCoolifyApplication
// and mapDokployApplication.
func mapServiceName(providerName, rawName string, issues []migrationIssue) (string, []migrationIssue) {
	sanitized := sanitizeServiceName(rawName)
	if sanitized != strings.ToLower(strings.TrimSpace(rawName)) {
		issues = appendIssue(issues, "name", issueReview, fmt.Sprintf(
			"%s application name %q was sanitized to %q to satisfy Levelrail's service-name pattern (lowercase alphanumeric and hyphens, starting with a letter)",
			providerName, rawName, sanitized))
	}
	return sanitized, issues
}

// mapCoolifyApplication translates one Coolify application (plus its env
// vars) into a Levelrail service. Pure: no I/O, no network, fully
// covered by table-driven tests (coolify_map_test.go).
func mapCoolifyApplication(app coolifyApplication, envs []coolifyEnvVar, includeSecretValues bool) mappedApp {
	out := mappedApp{
		SourceName: app.Name,
		Ref:        app.GitBranch,
	}

	out.RepoURL, out.Issues = sanitizeRepoForReport(app.GitRepository, out.Issues)
	out.ServiceName, out.Issues = mapServiceName("Coolify", app.Name, out.Issues)
	if app.BuildPack == "dockercompose" {
		return mapCoolifyComposeApp(app, envs, includeSecretValues)
	}

	buildType, buildIssue, blocking := mapBuildPack(app.BuildPack)
	if buildIssue != nil {
		out.Issues = append(out.Issues, *buildIssue)
	}
	if blocking {
		out.Blocking = true
		return out
	}
	out.Service.BuildType = buildType

	out.Service.Domains, out.Issues = mapDomains(app.FQDN, out.Issues)
	out.Service.Port, out.Issues = mapPort(app.PortsExposes, buildType, out.Issues)
	out.Service.Health, out.Issues = mapHealth(app, out.Issues)
	out.Service.Memory, out.Issues = mapMemory(app.LimitsMemory, out.Issues)
	out.Service.CPU, out.Issues = mapCPU(app.LimitsCPUs, out.Issues)
	out.Service.EnvKeys, out.Service.EnvLiteral, out.Issues = mapEnv(envs, includeSecretValues, out.Issues)
	out.Issues = mapBuildSettings(app, envs, &out.Service, out.Issues)
	out.Issues = mapReportOnly(app, out.ServiceName, out.Issues)
	out.Service.Ingress, out.Issues = mapIngress(app, out.ServiceName, out.Service.Domains, out.Issues)
	if app.PreviewsEnabled {
		out.Service.Previews = &mappedPreviews{}
		out.Issues = appendIssue(out.Issues, "is_preview_deployments_enabled", issuePlanned, "preview deployments were on in the source: previews are turned on with the default policy, review its limits before the first pull request")
	}
	out.SourceServer = app.Destination.Server.Name
	if out.SourceServer == "" {
		out.SourceServer = app.ServerName
	}
	if n := strings.TrimSpace(app.Destination.Network); n != "" && n != "coolify" {
		out.network = n
	}
	return out
}

// mapIngress translates basic auth, the www rule and proxy labels; unknown labels stay as dropped issues.
func mapIngress(app coolifyApplication, serviceName string, domains []string, issues []migrationIssue) (*platformimport.IngressPlan, []migrationIssue) {
	first := ""
	if len(domains) > 0 {
		first = domains[0]
	}
	p := platformimport.TranslateIngress(platformimport.IngressSource{Labels: app.CustomLabels, BasicAuth: app.BasicAuthEnabled,
		Redirect: app.Redirect, App: serviceName, Domain: first}, nil)
	for _, n := range p.Manual {
		issues = appendIssue(issues, "custom_labels", issueDropped, n.Reason+": "+n.Manual)
	}
	if !p.Translated() {
		return nil, issues
	}
	if p.BasicAuth {
		issues = appendIssue(issues, "is_http_basic_auth_enabled", issueReview, fmt.Sprintf("HTTP basic auth was on in the source: the app is public until basic auth is set. --apply gates each domain with a random placeholder password that is never shown; set the real one with domains basic-auth set %s <domain> (file mode sets nothing)", serviceName))
	}
	if p.Canonical != "" || p.ForceHTTPS || len(p.Forwarders) > 0 || len(p.Headers) > 0 {
		issues = appendIssue(issues, "ingress", issuePlanned, fmt.Sprintf("redirects and headers from the source proxy are set on the first domain by --apply (canonical %q, force https %t, %d forwarder and %d header rule(s))",
			p.Canonical, p.ForceHTTPS, len(p.Forwarders), len(p.Headers)))
	}
	return &p, issues
}

// linkNetworkPeers records, for each app on a custom network, the other apps on that network.
func linkNetworkPeers(apps []mappedApp) {
	byNet := map[string][]string{}
	for _, a := range apps {
		if a.network != "" && !a.Blocking {
			byNet[a.network] = append(byNet[a.network], a.ServiceName)
		}
	}
	for i := range apps {
		a := &apps[i]
		members := byNet[a.network]
		if a.network == "" || len(members) < 2 {
			continue
		}
		for _, m := range members {
			if m != a.ServiceName {
				a.NetworkPeers = append(a.NetworkPeers, m)
			}
		}
		a.Issues = appendIssue(a.Issues, "network", issuePlanned, fmt.Sprintf("shared a custom docker network with %s: put them in one project and use the service name or internal host as the address; no connection is created automatically", strings.Join(a.NetworkPeers, ", ")))
	}
}

// sourceCommand validates one Coolify command; a rejected one is reported by name only, its text may hold a secret.
func sourceCommand(field, raw string, values []string, issues []migrationIssue) (string, []migrationIssue) {
	cmd, err := platformimport.CleanSourceCommand(field, raw, values...)
	if err != nil {
		issues = appendIssue(issues, field, issuePlanned, err.Error()+", so it is not carried over; set it by hand and keep any secret in an app secret referenced as $NAME")
	}
	return cmd, issues
}

// mapBuildSettings carries base directory, commands, publish directory, deploy hooks and build-time keys.
func mapBuildSettings(app coolifyApplication, envs []coolifyEnvVar, svc *mappedService, issues []migrationIssue) []migrationIssue {
	base, err := platformimport.CleanSourcePath("base_directory", app.BaseDirectory)
	if err != nil {
		issues = appendIssue(issues, "base_directory", issueDropped, err.Error()+", the repository root is used")
	}
	svc.BaseDirectory = base
	// The server reads build.path from the repository root, while Coolify keeps it relative to the base directory.
	if loc, err := platformimport.CleanSourcePath("dockerfile_location", app.DockerfileLocation); err != nil {
		issues = appendIssue(issues, "dockerfile_location", issueDropped, err.Error()+", the default Dockerfile path is used")
	} else if full := path.Join(base, loc); loc != "" && full != "Dockerfile" && svc.BuildType == spec.BuildDockerfile {
		svc.BuildPath = full
	}
	values := make([]string, 0, 2*len(envs))
	for _, e := range envs {
		values = append(values, e.RealValue, e.Value)
	}
	var install, build, start, pre, post string
	install, issues = sourceCommand("install_command", app.InstallCommand, values, issues)
	build, issues = sourceCommand("build_command", app.BuildCommand, values, issues)
	start, issues = sourceCommand("start_command", app.StartCommand, values, issues)
	pre, issues = sourceCommand("pre_deployment_command", app.PreDeployment, values, issues)
	post, issues = sourceCommand("post_deployment_command", app.PostDeployment, values, issues)
	if (pre != "" || post != "") && svc.BuildType == spec.BuildStatic {
		issues = appendIssue(issues, "deploy_commands", issueDropped, "pre and post deployment commands need a running container, a static site has none: run them in CI or the build command")
	} else if pre != "" || post != "" {
		svc.Hooks = &mappedHooks{PreDeploy: pre, PostDeploy: post}
	}

	publish, err := platformimport.CleanSourcePath("publish_directory", app.PublishDirectory)
	if err != nil {
		issues = appendIssue(issues, "publish_directory", issueDropped, err.Error())
	}
	if svc.BuildType == spec.BuildStatic {
		issues = mapStaticBuild(install, build, start, publish, svc, issues)
	} else {
		for _, c := range []struct{ field, val string }{{"install_command", install}, {"build_command", build}} {
			if c.val != "" {
				issues = appendIssue(issues, c.field, issuePlanned, fmt.Sprintf("%q is not applied to a %s build: Railpack detects the step itself and a Dockerfile defines its own; put it in the Dockerfile or the repository if the result differs", c.val, svc.BuildType))
			}
		}
		if start != "" {
			svc.StartCommand = start
		}
		if publish != "" {
			issues = appendIssue(issues, "publish_directory", issueDropped, "publish directory only applies to static builds")
		}
	}

	for _, e := range envs {
		if e.IsBuildTime || e.IsBuildtime {
			svc.BuildTimeKeys = append(svc.BuildTimeKeys, e.Key)
		}
	}
	for _, e := range envs {
		if slices.Contains(platformimport.NodePinKeys, e.Key) {
			issues = appendIssue(issues, "node_version", issuePlanned, fmt.Sprintf("the source pins Node through %s: pin it in the repository with engines.node in package.json or a .node-version file, which Railpack reads", e.Key))
			break
		}
	}
	sort.Strings(svc.BuildTimeKeys)
	if len(svc.BuildTimeKeys) > 0 {
		issues = appendIssue(issues, "environment_variables", issuePlanned, fmt.Sprintf("build-time variable(s) %s are stored as secrets with build access on apply; declare each as a build secret (RUN --mount=type=secret) or ARG where the build reads it", strings.Join(svc.BuildTimeKeys, ", ")))
	}
	return issues
}

// mapStaticBuild maps a static app: a build command needs a publish subdirectory, else committed files are served.
func mapStaticBuild(install, build, start, publish string, svc *mappedService, issues []migrationIssue) []migrationIssue {
	if start != "" {
		issues = appendIssue(issues, "start_command", issueDropped, "static sites are served directly, no start command runs")
	}
	cmd := build
	if install != "" && build != "" {
		cmd = install + " && " + build
	} else if install != "" {
		issues = appendIssue(issues, "install_command", issuePlanned, fmt.Sprintf("install command %q has no build command to run with, add the build step by hand", install))
	}
	switch {
	case cmd != "" && publish != "":
		svc.BuildCommand, svc.PublishDir = cmd, publish
	case cmd != "":
		issues = appendIssue(issues, "publish_directory", issuePlanned, fmt.Sprintf("build command %q needs a publish subdirectory, set build.publish to the folder the build writes", cmd))
	case publish != "":
		svc.BuildPath = path.Join(svc.BaseDirectory, publish)
	}
	return issues
}

// mapReportOnly lists settings with no direct field here, each with a reason, so none is dropped silently.
func mapReportOnly(app coolifyApplication, serviceName string, issues []migrationIssue) []migrationIssue {
	if strings.TrimSpace(app.DockerRunOptions) != "" {
		issues = appendIssue(issues, "custom_docker_run_options", issueDropped, "custom docker run options are not copied: translate each option into app settings by hand")
	}
	if app.WebhookGithub != "" || app.WebhookGitlab != "" || app.WebhookBitbucket != "" || app.WebhookGitea != "" {
		issues = appendIssue(issues, "deploy_webhook", issuePlanned, fmt.Sprintf("a deploy webhook secret is set in the source (never read into this report): run apps build-hooks create %s --name deploy, point the git host or CI at the new URL, then remove the old webhook", serviceName))
	}
	if tags := platformimport.ParseSourceTags(app.Tags); len(tags) > 0 {
		issues = appendIssue(issues, "tags", issuePlanned, fmt.Sprintf("source tags %s have no tag field here: group the apps in one project or give them a shared name prefix", strings.Join(tags, ", ")))
	}
	return issues
}

func mapBuildPack(buildPack string) (buildType string, issue *migrationIssue, blocking bool) {
	switch buildPack {
	case "dockerfile":
		return spec.BuildDockerfile, nil, false
	case "railpack":
		return spec.BuildRailpack, nil, false
	case "static":
		return spec.BuildStatic, nil, false
	case "nixpacks":
		return "", blockingIssue("build_pack", "Coolify's nixpacks build pack has no Levelrail equivalent (Levelrail uses Railpack, not Nixpacks); pick dockerfile, railpack, or static manually and create this app by hand"), true
	default:
		return "", blockingIssue("build_pack", fmt.Sprintf("unrecognized Coolify build_pack %q", buildPack)), true
	}
}

func mapDomains(fqdn string, issues []migrationIssue) ([]string, []migrationIssue) {
	fqdn = strings.TrimSpace(fqdn)
	if fqdn == "" {
		return nil, issues
	}
	var domains []string
	for _, part := range strings.Split(fqdn, ",") {
		part = strings.TrimSuffix(strings.TrimSpace(part), "/")
		if part == "" {
			continue
		}
		if u, err := url.Parse(part); err == nil && u.Host != "" {
			domains = append(domains, u.Host)
			continue
		}
		domains = append(domains, strings.TrimPrefix(strings.TrimPrefix(part, "https://"), "http://"))
	}
	return domains, issues
}

func mapPort(portsExposes, buildType string, issues []migrationIssue) (int, []migrationIssue) {
	portsExposes = strings.TrimSpace(portsExposes)
	if portsExposes == "" {
		return 0, issues
	}
	parts := strings.Split(portsExposes, ",")
	first := strings.TrimSpace(parts[0])
	port, err := strconv.Atoi(first)
	if err != nil {
		issues = appendIssue(issues, "ports_exposes", issueReview, fmt.Sprintf("could not parse a port from %q, port left unset, set it manually", portsExposes))
		return 0, issues
	}
	if buildType == spec.BuildStatic {
		issues = appendIssue(issues, "ports_exposes", issueDropped, fmt.Sprintf("ports_exposes %q ignored: static sites have no running container to route to", portsExposes))
		return 0, issues
	}
	if len(parts) > 1 {
		issues = appendIssue(issues, "ports_exposes", issueDropped, fmt.Sprintf("only the first port (%d) was migrated; app.yaml supports one port per service, additional port(s) %s were dropped", port, strings.Join(parts[1:], ",")))
	}
	return port, issues
}

func mapHealth(app coolifyApplication, issues []migrationIssue) (*mappedHealth, []migrationIssue) {
	if !app.HealthCheckEnabled {
		return nil, issues
	}
	if app.HealthCheckType == "cmd" || app.HealthCheckCommand != "" {
		issues = appendIssue(issues, "health_check", issueDropped, "Coolify's CMD-style health check has no Levelrail equivalent (Levelrail only supports HTTP-path health checks); no health check was migrated")
		return nil, issues
	}
	path := app.HealthCheckPath
	if path == "" {
		path = "/"
	}
	return &mappedHealth{
		Path:     path,
		Interval: app.HealthCheckInterval,
		Timeout:  app.HealthCheckTimeout,
		Failures: app.HealthCheckRetries,
	}, issues
}

var dockerMemoryPattern = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)\s*([a-zA-Z]*)$`)

// parseDockerMemoryBytes parses limits_memory's Docker-native shape
// (e.g. "512m", "1g", "0"; case-insensitive unit, no unit means bytes).
func parseDockerMemoryBytes(s string) (int64, error) {
	m := dockerMemoryPattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("invalid docker memory value %q", s)
	}
	num, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid docker memory value %q: %w", s, err)
	}
	var multiplier float64
	switch strings.ToLower(m[2]) {
	case "", "b":
		multiplier = 1
	case "k", "kb":
		multiplier = 1024
	case "m", "mb":
		multiplier = 1024 * 1024
	case "g", "gb":
		multiplier = 1024 * 1024 * 1024
	case "t", "tb":
		multiplier = 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unrecognized docker memory unit %q in %q", m[2], s)
	}
	return int64(num * multiplier), nil
}

// Rounds up to whole Mi: under-reporting a migrated app's memory limit is the worse failure.
func formatMemoryMiGi(bytes int64) string {
	const mi = 1024 * 1024
	const gi = mi * 1024
	if bytes%gi == 0 {
		return fmt.Sprintf("%dGi", bytes/gi)
	}
	mib := (bytes + mi - 1) / mi
	return fmt.Sprintf("%dMi", mib)
}

type numericLimit interface {
	~int64 | ~float64
}

// The two providers disagree on whether a non-positive value is a review issue, so this is a parameter.
func parseNumericLimit[T numericLimit](raw, field string, issues []migrationIssue, parse func(string) (T, error), issueOnNonPositive bool, detail func(raw string, err error) string) (T, []migrationIssue) {
	var zero T
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" {
		return zero, issues
	}
	v, err := parse(raw)
	if err == nil && issueOnNonPositive && v <= 0 {
		err = fmt.Errorf("non-positive value")
	}
	if err != nil {
		issues = appendIssue(issues, field, issueReview, detail(raw, err))
		return zero, issues
	}
	if v <= 0 {
		return zero, issues
	}
	return v, issues
}

func mapMemory(raw string, issues []migrationIssue) (string, []migrationIssue) {
	bytes, issues := parseNumericLimit(raw, "limits_memory", issues, parseDockerMemoryBytes, false,
		func(raw string, err error) string {
			return fmt.Sprintf("could not parse limits_memory %q, resources.memory left unset: %v", raw, err)
		})
	if bytes == 0 {
		return "", issues
	}
	return formatMemoryMiGi(bytes), issues
}

func mapCPU(raw string, issues []migrationIssue) (float64, []migrationIssue) {
	return parseNumericLimit(raw, "limits_cpus", issues, func(s string) (float64, error) { return strconv.ParseFloat(s, 64) }, false,
		func(raw string, err error) string {
			return fmt.Sprintf("could not parse limits_cpus %q, resources.cpu left unset: %v", raw, err)
		})
}

// Values are populated only when includeSecretValues is set and Coolify returned one.
func mapEnv(envs []coolifyEnvVar, includeSecretValues bool, issues []migrationIssue) ([]string, map[string]string, []migrationIssue) {
	if len(envs) == 0 {
		return nil, nil, issues
	}
	keys := make([]string, 0, len(envs))
	for _, e := range envs {
		keys = append(keys, e.Key)
	}
	sort.Strings(keys)

	if !includeSecretValues {
		issues = appendIssue(issues, "environment_variables", issueReview, fmt.Sprintf("%d env var(s) migrated as key-only secret placeholders; pass --include-secret-values with a Coolify token that has the read:sensitive ability to fetch real values", len(keys)))
		return keys, nil, issues
	}

	literal := make(map[string]string, len(envs))
	var missing []string
	for _, e := range envs {
		v := e.RealValue
		if v == "" {
			v = e.Value
		}
		if v == "" {
			missing = append(missing, e.Key)
			continue
		}
		literal[e.Key] = v
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		issues = appendIssue(issues, "environment_variables", issueReview, fmt.Sprintf("no value returned by Coolify for %s even with --include-secret-values; the Coolify token likely lacks the read:sensitive ability", strings.Join(missing, ", ")))
	}
	return keys, literal, issues
}

// stagedCompose is a one-click service translated to a compose app. Yaml holds
// generated secrets, so it is never serialized into the report.
type stagedCompose struct {
	Yaml        string   `json:"-"`
	Generated   []string `json:"generated_secrets,omitempty"`
	Hosts       []string `json:"hosts,omitempty"`
	Volumes     []string `json:"volumes,omitempty"`
	Unsupported []string `json:"unsupported,omitempty"`
	StagedPath  string   `json:"staged_path,omitempty"`
	Deploy      string   `json:"deploy_command,omitempty"`
}

var (
	composeRefPattern  = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	composeVolumeEntry = regexp.MustCompile(`(?m)^\s*-\s+["']?([^\s"':]+):([^\s"']+)["']?\s*$`)
	coolifyOnlyKey     = regexp.MustCompile(`(?m)^\s*(exclude_from_hc|x-coolify[A-Za-z0-9_-]*):|^\s*-\s+["']?coolify\.[^\s"']*`)
)

// mapCoolifyService translates a discovered one-click service into a compose
// app that can be deployed later. domainSuffix is the parent domain for the
// SERVICE_FQDN_* hosts and defaults to "localhost" so nothing routes publicly.
func mapCoolifyService(s platformimport.Service, domainSuffix string) mappedApp {
	var issues []migrationIssue
	name, issues := mapServiceName("Coolify", s.Name, issues)
	out := mappedApp{SourceName: s.Name, ServiceName: name}

	if strings.TrimSpace(s.Compose) == "" {
		out.Issues = appendIssue(issues, "compose", issueBlocking, "the service has no readable compose body, export it from Coolify and deploy it manually")
		out.Blocking = true
		return out
	}
	if domainSuffix == "" {
		domainSuffix = "localhost"
	}
	host := func(key string) string {
		return sanitizeServiceName(key) + "." + domainSuffix
	}
	tr, err := compose.TranslateMagicVars(s.Compose, host)
	if err != nil {
		out.Issues = appendIssue(issues, "compose", issueBlocking, fmt.Sprintf("could not generate a secret: %v", err))
		out.Blocking = true
		return out
	}

	if blocked := platformimport.HostAccessBlocked(tr.Compose); len(blocked) > 0 {
		for _, r := range blocked {
			issues = appendIssue(issues, "compose", issueBlocking, r)
		}
		out.Issues, out.Blocking = issues, true
		return out
	}
	if _, rest := platformimport.CheckCompose(relativeBindsAsVolumes(tr.Compose)); len(rest) > 0 {
		for _, r := range rest {
			issues = appendIssue(issues, "compose", issueReview, "the compose deploy rejects this until fixed in the staged file: "+r)
		}
	}
	sc := &stagedCompose{Yaml: tr.Compose, Generated: tr.Generated, Hosts: tr.Hosts, Unsupported: append([]string(nil), tr.Unsupported...)}
	for _, tok := range tr.Unsupported {
		issues = appendIssue(issues, tok, issueDropped, "magic variable has no Levelrail equivalent and was left in place, set a real value before deploying")
	}
	for _, m := range composeVolumeEntry.FindAllStringSubmatch(tr.Compose, -1) {
		src := m[1]
		if strings.HasPrefix(src, ".") || strings.HasPrefix(src, "/") || strings.HasPrefix(src, "~") {
			sc.Unsupported = append(sc.Unsupported, "volume "+src+":"+m[2])
			issues = appendIssue(issues, "volumes", issueReview, fmt.Sprintf("bind mount %q is a path on the Coolify server, copy its data over and use a named volume", src))
			continue
		}
		sc.Volumes = append(sc.Volumes, src)
	}
	for _, m := range coolifyOnlyKey.FindAllString(tr.Compose, -1) {
		key := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(m), "- "))
		sc.Unsupported = append(sc.Unsupported, key)
		issues = appendIssue(issues, "compose", issueReview, fmt.Sprintf("Coolify only key %q is ignored", strings.Trim(key, `:"'`)))
	}
	if strings.Contains(tr.Compose, platformimport.SecretMask) {
		sc.Unsupported = append(sc.Unsupported, "masked secret values")
		issues = appendIssue(issues, "secrets", issueReview, "inline secret values were masked by the source read, replace "+platformimport.SecretMask+" with real values before deploying")
	}
	var refs []string
	for _, m := range composeRefPattern.FindAllStringSubmatch(tr.Compose, -1) {
		refs = append(refs, m[1])
	}
	sort.Strings(refs)
	for i, r := range refs {
		if i > 0 && refs[i-1] == r {
			continue
		}
		issues = appendIssue(issues, r, issueReview, "compose references ${"+r+"} with no default, supply it in the app env before deploying")
	}
	sort.Strings(sc.Unsupported)
	out.Compose = sc
	out.Issues = issues
	return out
}

// relativeBindsAsVolumes swaps relative bind sources for placeholder volumes in a check copy only:
// they are reported for review below, and the host-access rules must still run on the rest.
func relativeBindsAsVolumes(yaml string) string {
	out := yaml
	for i, m := range composeVolumeEntry.FindAllStringSubmatch(yaml, -1) {
		if strings.HasPrefix(m[1], ".") || strings.HasPrefix(m[1], "~") {
			out = strings.Replace(out, m[0], strings.Replace(m[0], m[1], "import-bind-"+strconv.Itoa(i), 1), 1)
		}
	}
	return out
}

// hasRawMagicVar reports whether a SERVICE_ token survived translation, which
// the compose deploy would reject.
func hasRawMagicVar(yaml string) bool {
	return len(compose.FindMagicVars(yaml)) > 0
}

// mappedFile is a config file copied from the source; Content is never serialized or printed.
type mappedFile struct {
	Path       string `json:"path"`
	Secret     bool   `json:"secret,omitempty"`
	Content    []byte `json:"-"`
	StagedPath string `json:"staged_path,omitempty"`
}

// mapCoolifyComposeApp stages a dockercompose build pack app like a one-click service, with
// generated secrets resolved and the same compose validation a manual deploy runs.
func mapCoolifyComposeApp(app coolifyApplication, envs []coolifyEnvVar, includeSecretValues bool) mappedApp {
	out := mapCoolifyService(platformimport.Service{SourceID: app.UUID, Name: app.Name, Compose: app.DockerComposeRaw}, "")
	out.RepoURL, out.Ref = app.GitRepository, app.GitBranch
	if out.Blocking {
		return out
	}
	out.Service.Domains, out.Issues = mapDomains(app.FQDN, out.Issues)
	out.Service.EnvKeys, out.Service.EnvLiteral, out.Issues = mapEnv(envs, includeSecretValues, out.Issues)
	return out
}

// mapFileStorages copies file mounts byte for byte, only when real values were requested since
// any config file may hold a credential. Refused, duplicate and nested paths stay out with a reason.
func mapFileStorages(st coolifyStorageList, includeSecretValues bool, issues []migrationIssue) ([]mappedFile, []migrationIssue) {
	var out []mappedFile
	seen := map[string]bool{}
	for _, fs := range st.Files {
		if fs.IsDirectory {
			issues = appendIssue(issues, "file_storages", issueReview, "directory mount "+fs.MountPath+" is not copied, recreate its files with apps files set")
			continue
		}
		f, err := platformimport.NewAppFile(fs.MountPath, []byte(fs.Content))
		if err != nil {
			issues = appendIssue(issues, "file_storages", issueDropped, "file mount refused: "+err.Error())
			continue
		}
		if seen[f.Path] {
			issues = appendIssue(issues, "file_storages", issueDropped, "file "+f.Path+" is mounted twice, only the first is kept")
			continue
		}
		seen[f.Path] = true
		if !includeSecretValues {
			issues = appendIssue(issues, "file_storages", issueReview, fmt.Sprintf("file %s (%d bytes) was not copied, re-run with --include-secret-values or set it with apps files set", f.Path, len(f.Content)))
			continue
		}
		out = append(out, mappedFile{Path: f.Path, Secret: f.Secret, Content: f.Content})
	}
	return dropNestedFiles(out, issues)
}

// dropNestedFiles refuses a file whose path sits under another file's path, which no directory tree can hold.
func dropNestedFiles(files []mappedFile, issues []migrationIssue) ([]mappedFile, []migrationIssue) {
	var out []mappedFile
	for _, f := range files {
		nested := false
		for _, g := range files {
			nested = nested || strings.HasPrefix(f.Path, g.Path+"/")
		}
		if nested {
			issues = appendIssue(issues, "file_storages", issueDropped, "file "+f.Path+" sits under another mounted file and was not copied")
			continue
		}
		out = append(out, f)
	}
	return out, issues
}
