package main

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// issueSeverity classifies a migrationIssue for report grouping.
type issueSeverity string

const (
	issueDropped  issueSeverity = "dropped"  // has no Levelrail equivalent, silently lost otherwise
	issueReview   issueSeverity = "review"   // migrated with a caveat the operator should check
	issueBlocking issueSeverity = "blocking" // the whole app could not be mapped at all
)

type migrationIssue struct {
	Field    string        `json:"field"`
	Severity issueSeverity `json:"severity"`
	Detail   string        `json:"detail"`
}

// mappedApp is one Coolify application's migration result: either a
// generated service (Blocking false) or a reason it can't be migrated at
// all (Blocking true, e.g. build_pack: dockercompose), plus every issue
// found along the way either way.
type mappedApp struct {
	SourceName  string           `json:"source_name"`
	ServiceName string           `json:"service_name,omitempty"`
	RepoURL     string           `json:"repo_url,omitempty"`
	Ref         string           `json:"ref,omitempty"`
	Service     mappedService    `json:"service"`
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
	// EnvLiteral holds a real fetched value for a subset of EnvKeys, set
	// only when includeSecretValues was requested and Coolify returned
	// one. Never serialized: a --json migration report must never leak
	// live secret material, and never written into a generated app.yaml
	// either (see coolify_yaml.go's buildAppYAML doc comment); it is only
	// ever consumed by writeSecretsEnvFile or applyMigration's
	// PUT .../secrets/{key} calls.
	EnvLiteral map[string]string `json:"-"`
}

type mappedHealth struct {
	Path     string `json:"path"`
	Interval int    `json:"interval_seconds,omitempty"`
	Timeout  int    `json:"timeout_seconds,omitempty"`
	Failures int    `json:"failures,omitempty"`
}

var invalidNameChars = regexp.MustCompile(`[^a-z0-9-]+`)

// sanitizeServiceName turns a Coolify application name into something
// internal/spec's own nameLike pattern accepts (lowercase alphanumeric
// and hyphens, starting with a letter): Coolify names are free text, an
// app.yaml service key is not.
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

// mapCoolifyApplication translates one Coolify application (plus its env
// vars) into a Levelrail service. Pure: no I/O, no network, fully
// covered by table-driven tests (coolify_map_test.go).
func mapCoolifyApplication(app coolifyApplication, envs []coolifyEnvVar, includeSecretValues bool) mappedApp {
	out := mappedApp{
		SourceName: app.Name,
		RepoURL:    app.GitRepository,
		Ref:        app.GitBranch,
	}

	sanitized := sanitizeServiceName(app.Name)
	out.ServiceName = sanitized
	if sanitized != strings.ToLower(strings.TrimSpace(app.Name)) {
		out.Issues = append(out.Issues, migrationIssue{
			Field: "name", Severity: issueReview,
			Detail: fmt.Sprintf("Coolify application name %q was sanitized to %q to satisfy Levelrail's service-name pattern (lowercase alphanumeric and hyphens, starting with a letter)", app.Name, sanitized),
		})
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
	if loc := strings.TrimPrefix(strings.TrimSpace(app.DockerfileLocation), "/"); loc != "" && loc != "Dockerfile" && buildType == spec.BuildDockerfile {
		out.Service.BuildPath = loc
	}

	out.Service.Domains, out.Issues = mapDomains(app.FQDN, out.Issues)
	out.Service.Port, out.Issues = mapPort(app.PortsExposes, buildType, out.Issues)
	out.Service.Health, out.Issues = mapHealth(app, out.Issues)
	out.Service.Memory, out.Issues = mapMemory(app.LimitsMemory, out.Issues)
	out.Service.CPU, out.Issues = mapCPU(app.LimitsCPUs, out.Issues)
	out.Service.EnvKeys, out.Service.EnvLiteral, out.Issues = mapEnv(envs, includeSecretValues, out.Issues)

	return out
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
		return "", &migrationIssue{
			Field: "build_pack", Severity: issueBlocking,
			Detail: "Coolify's nixpacks build pack has no Levelrail equivalent (Levelrail uses Railpack, not Nixpacks); pick dockerfile, railpack, or static manually and create this app by hand",
		}, true
	case "dockercompose":
		return "", &migrationIssue{
			Field: "build_pack", Severity: issueBlocking,
			Detail: "Coolify's dockercompose build pack is multi-service; Levelrail does not yet support Compose as a deploy target, and this tool does not attempt to translate the compose file",
		}, true
	default:
		return "", &migrationIssue{
			Field: "build_pack", Severity: issueBlocking,
			Detail: fmt.Sprintf("unrecognized Coolify build_pack %q", buildPack),
		}, true
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
		issues = append(issues, migrationIssue{
			Field: "ports_exposes", Severity: issueReview,
			Detail: fmt.Sprintf("could not parse a port from %q, port left unset, set it manually", portsExposes),
		})
		return 0, issues
	}
	if buildType == spec.BuildStatic {
		issues = append(issues, migrationIssue{
			Field: "ports_exposes", Severity: issueDropped,
			Detail: fmt.Sprintf("ports_exposes %q ignored: static sites have no running container to route to", portsExposes),
		})
		return 0, issues
	}
	if len(parts) > 1 {
		issues = append(issues, migrationIssue{
			Field: "ports_exposes", Severity: issueDropped,
			Detail: fmt.Sprintf("only the first port (%d) was migrated; app.yaml supports one port per service, additional port(s) %s were dropped", port, strings.Join(parts[1:], ",")),
		})
	}
	return port, issues
}

func mapHealth(app coolifyApplication, issues []migrationIssue) (*mappedHealth, []migrationIssue) {
	if !app.HealthCheckEnabled {
		return nil, issues
	}
	if app.HealthCheckType == "cmd" || app.HealthCheckCommand != "" {
		issues = append(issues, migrationIssue{
			Field: "health_check", Severity: issueDropped,
			Detail: "Coolify's CMD-style health check has no Levelrail equivalent (Levelrail only supports HTTP-path health checks); no health check was migrated",
		})
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

// formatMemoryMiGi mirrors internal/api/builds.go's own formatMemoryBytes
// (Gi when it divides evenly, Mi otherwise), except it rounds up to the
// nearest whole Mi rather than truncating: under-reporting a migrated
// app's resource limit is a worse failure mode than over-reporting it.
func formatMemoryMiGi(bytes int64) string {
	const mi = 1024 * 1024
	const gi = mi * 1024
	if bytes%gi == 0 {
		return fmt.Sprintf("%dGi", bytes/gi)
	}
	mib := (bytes + mi - 1) / mi
	return fmt.Sprintf("%dMi", mib)
}

func mapMemory(raw string, issues []migrationIssue) (string, []migrationIssue) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" {
		return "", issues
	}
	bytes, err := parseDockerMemoryBytes(raw)
	if err != nil {
		issues = append(issues, migrationIssue{
			Field: "limits_memory", Severity: issueReview,
			Detail: fmt.Sprintf("could not parse limits_memory %q, resources.memory left unset: %v", raw, err),
		})
		return "", issues
	}
	if bytes <= 0 {
		return "", issues
	}
	return formatMemoryMiGi(bytes), issues
}

func mapCPU(raw string, issues []migrationIssue) (float64, []migrationIssue) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" {
		return 0, issues
	}
	cpu, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		issues = append(issues, migrationIssue{
			Field: "limits_cpus", Severity: issueReview,
			Detail: fmt.Sprintf("could not parse limits_cpus %q, resources.cpu left unset: %v", raw, err),
		})
		return 0, issues
	}
	if cpu <= 0 {
		return 0, issues
	}
	return cpu, issues
}

// mapEnv always returns every Coolify env var's key. Real values are
// only ever populated in the returned map when includeSecretValues is
// set and Coolify actually returned one; see coolifyEnvVar's own doc
// comment for why that's not guaranteed even when asked for.
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
		issues = append(issues, migrationIssue{
			Field: "environment_variables", Severity: issueReview,
			Detail: fmt.Sprintf("%d env var(s) migrated as key-only secret placeholders; pass --include-secret-values with a Coolify token that has the read:sensitive ability to fetch real values", len(keys)),
		})
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
		issues = append(issues, migrationIssue{
			Field: "environment_variables", Severity: issueReview,
			Detail: fmt.Sprintf("no value returned by Coolify for %s even with --include-secret-values; the Coolify token likely lacks the read:sensitive ability", strings.Join(missing, ", ")),
		})
	}
	return keys, literal, issues
}
