package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// mapDokployApplication translates one Dokploy application (plus its
// domains) into a Levelrail service. Pure: no I/O, no network, fully
// covered by table-driven tests (dokploy_map_test.go). Reuses mappedApp,
// mappedService, migrationIssue, issueSeverity, and sanitizeServiceName
// from coolify_map.go: both migration sources produce the same report
// and app.yaml shape.
func mapDokployApplication(app dokployApplication, domains []dokployDomain, includeSecretValues bool) mappedApp {
	out := mappedApp{SourceName: app.Name}

	sanitized := sanitizeServiceName(app.Name)
	out.ServiceName = sanitized
	if sanitized != strings.ToLower(strings.TrimSpace(app.Name)) {
		out.Issues = append(out.Issues, migrationIssue{
			Field: "name", Severity: issueReview,
			Detail: fmt.Sprintf("Dokploy application name %q was sanitized to %q to satisfy Levelrail's service-name pattern (lowercase alphanumeric and hyphens, starting with a letter)", app.Name, sanitized),
		})
	}

	buildType, buildPath, buildIssue, blocking := mapDokployBuildConfig(app)
	if buildIssue != nil {
		out.Issues = append(out.Issues, *buildIssue)
	}
	if blocking {
		out.Blocking = true
		return out
	}
	out.Service.BuildType = buildType
	out.Service.BuildPath = buildPath

	out.RepoURL, out.Ref, out.Issues = mapDokployRepo(app, out.Issues)
	out.Service.Domains, out.Service.Port, out.Issues = mapDokployDomains(domains, buildType, out.Issues)
	out.Service.Memory, out.Issues = mapDokployMemory(app.MemoryLimit, out.Issues)
	out.Service.CPU, out.Issues = mapDokployCPU(app.CPULimit, out.Issues)
	out.Service.EnvKeys, out.Service.EnvLiteral, out.Issues = mapDokployEnv(app.Env, includeSecretValues, out.Issues)

	return out
}

// mapDokployBuildConfig has no direct Levelrail equivalent for Dokploy's
// "docker" (prebuilt image) and "drop" (file upload) source types today:
// mappedService carries no image field, only build.type/build.path (see
// coolify_map.go's mappedService), so those sources are reported as
// needing manual review rather than partially mapped.
func mapDokployBuildConfig(app dokployApplication) (buildType, buildPath string, issue *migrationIssue, blocking bool) {
	switch app.SourceType {
	case "docker":
		return "", "", &migrationIssue{
			Field: "sourceType", Severity: issueBlocking,
			Detail: "Dokploy's docker (prebuilt image) source type has no app.yaml build path today; create this app manually with the target image",
		}, true
	case "drop":
		return "", "", &migrationIssue{
			Field: "sourceType", Severity: issueBlocking,
			Detail: "Dokploy's drop (file upload) source type has no git or image source Levelrail can use; create this app manually",
		}, true
	}

	switch app.BuildType {
	case "dockerfile":
		return spec.BuildDockerfile, cleanDokployPath(app.Dockerfile), nil, false
	case "railpack":
		return spec.BuildRailpack, "", nil, false
	case "static":
		return spec.BuildStatic, "", nil, false
	case "nixpacks":
		return "", "", &migrationIssue{
			Field: "buildType", Severity: issueBlocking,
			Detail: "Dokploy's nixpacks build type has no Levelrail equivalent (Levelrail uses Railpack, not Nixpacks); pick dockerfile, railpack, or static manually and create this app by hand",
		}, true
	case "heroku_buildpacks", "paketo_buildpacks":
		return "", "", &migrationIssue{
			Field: "buildType", Severity: issueBlocking,
			Detail: fmt.Sprintf("Dokploy's %s build type has no Levelrail equivalent; pick dockerfile, railpack, or static manually and create this app by hand", app.BuildType),
		}, true
	default:
		return "", "", &migrationIssue{
			Field: "buildType", Severity: issueBlocking,
			Detail: fmt.Sprintf("unrecognized Dokploy build_type %q", app.BuildType),
		}, true
	}
}

func cleanDokployPath(loc string) string {
	loc = strings.TrimPrefix(strings.TrimSpace(loc), "/")
	if loc == "" || loc == "Dockerfile" {
		return ""
	}
	return loc
}

// mapDokployRepo builds a repo URL only for providers with an
// unambiguous public host (github.com, bitbucket.org, or a custom git
// URL that is already a full URL). GitLab and Gitea are commonly
// self-hosted and Dokploy's API exposes only owner/repository/branch for
// them, not the instance host, so those are left for manual review
// rather than guessing a host that may be wrong.
func mapDokployRepo(app dokployApplication, issues []migrationIssue) (repoURL, ref string, out []migrationIssue) {
	out = issues
	switch app.SourceType {
	case "github":
		if app.Owner != "" && app.Repository != "" {
			return fmt.Sprintf("https://github.com/%s/%s.git", app.Owner, app.Repository), app.Branch, out
		}
	case "bitbucket":
		if app.BitbucketOwner != "" && app.BitbucketRepository != "" {
			return fmt.Sprintf("https://bitbucket.org/%s/%s.git", app.BitbucketOwner, app.BitbucketRepository), app.BitbucketBranch, out
		}
	case "git":
		if app.CustomGitURL != "" {
			return app.CustomGitURL, app.CustomGitBranch, out
		}
	case "gitlab":
		if app.GitlabOwner != "" && app.GitlabRepository != "" {
			out = append(out, migrationIssue{
				Field: "repository", Severity: issueReview,
				Detail: fmt.Sprintf("Dokploy's API does not expose the GitLab instance host (often self-hosted); owner=%q repository=%q branch=%q, set the repo URL manually", app.GitlabOwner, app.GitlabRepository, app.GitlabBranch),
			})
		}
	case "gitea":
		if app.GiteaOwner != "" && app.GiteaRepository != "" {
			out = append(out, migrationIssue{
				Field: "repository", Severity: issueReview,
				Detail: fmt.Sprintf("Dokploy's API does not expose the Gitea instance host (Gitea is always self-hosted); owner=%q repository=%q branch=%q, set the repo URL manually", app.GiteaOwner, app.GiteaRepository, app.GiteaBranch),
			})
		}
	}
	return "", "", out
}

func mapDokployDomains(domains []dokployDomain, buildType string, issues []migrationIssue) ([]string, int, []migrationIssue) {
	if len(domains) == 0 {
		return nil, 0, issues
	}

	hosts := make([]string, 0, len(domains))
	for _, d := range domains {
		if d.Host != "" {
			hosts = append(hosts, d.Host)
		}
	}

	port := domains[0].Port
	if buildType == spec.BuildStatic {
		if port != 0 {
			issues = append(issues, migrationIssue{
				Field: "domains", Severity: issueDropped,
				Detail: fmt.Sprintf("domain port %d ignored: static sites have no running container to route to", port),
			})
		}
		return hosts, 0, issues
	}

	for _, d := range domains[1:] {
		if d.Port != 0 && d.Port != port {
			issues = append(issues, migrationIssue{
				Field: "domains", Severity: issueReview,
				Detail: fmt.Sprintf("domain %q declares port %d, different from the first domain's port %d; app.yaml supports one port per service, only %d was migrated", d.Host, d.Port, port, port),
			})
		}
	}
	return hosts, port, issues
}

func mapDokployMemory(raw string, issues []migrationIssue) (string, []migrationIssue) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" {
		return "", issues
	}
	bytes, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || bytes <= 0 {
		issues = append(issues, migrationIssue{
			Field: "memoryLimit", Severity: issueReview,
			Detail: fmt.Sprintf("could not parse memoryLimit %q as a byte count, resources.memory left unset", raw),
		})
		return "", issues
	}
	return formatMemoryMiGi(bytes), issues
}

func mapDokployCPU(raw string, issues []migrationIssue) (float64, []migrationIssue) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" {
		return 0, issues
	}
	nanocpus, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || nanocpus <= 0 {
		issues = append(issues, migrationIssue{
			Field: "cpuLimit", Severity: issueReview,
			Detail: fmt.Sprintf("could not parse cpuLimit %q as a nanoCPU count, resources.cpu left unset", raw),
		})
		return 0, issues
	}
	return float64(nanocpus) / 1e9, issues
}

// parseDokployEnv parses Dokploy's single dotenv-style env blob into a
// key/value map. Blank lines and lines starting with # are skipped.
func parseDokployEnv(raw string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return out
}

// mapDokployEnv always returns every parsed key. Unlike Coolify, Dokploy
// has no documented redaction ability gate: application.one's env field
// already carries real values to any caller with access to the app, so
// includeSecretValues here only controls whether this tool writes/applies
// those values, not whether Dokploy returned them.
func mapDokployEnv(raw string, includeSecretValues bool, issues []migrationIssue) ([]string, map[string]string, []migrationIssue) {
	parsed := parseDokployEnv(raw)
	if len(parsed) == 0 {
		return nil, nil, issues
	}

	keys := make([]string, 0, len(parsed))
	for k := range parsed {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if !includeSecretValues {
		issues = append(issues, migrationIssue{
			Field: "env", Severity: issueReview,
			Detail: fmt.Sprintf("%d env var(s) migrated as key-only secret placeholders; pass --include-secret-values to write the real values already returned by Dokploy into a companion secrets file or apply them via PUT .../secrets/{key}", len(keys)),
		})
		return keys, nil, issues
	}
	return keys, parsed, issues
}
