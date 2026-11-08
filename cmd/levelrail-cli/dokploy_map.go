package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// Pure mapping with no I/O; reuses coolify_map.go's mapped types so both sources emit the same report.
func mapDokployApplication(app dokployApplication, domains []dokployDomain, includeSecretValues bool) mappedApp {
	out := mappedApp{SourceName: app.Name}

	out.ServiceName, out.Issues = mapServiceName("Dokploy", app.Name, out.Issues)

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

	out.Service.Previews, out.Issues = mapDokployPreviews(app, out.Issues)

	return out
}

// Only the limit maps onto the preview policy; the rest is reported so nothing drops silently.
func mapDokployPreviews(app dokployApplication, issues []migrationIssue) (*mappedPreviews, []migrationIssue) {
	if app.AutoDeploy != nil && !*app.AutoDeploy {
		issues = appendIssue(issues, "autoDeploy", issueReview, "auto deploy on push is off in Dokploy; the new app builds only when you start a deploy, keep it that way")
	}
	if app.CleanCache {
		issues = appendIssue(issues, "cleanCache", issueReview, "Dokploy cleans the build cache on every deploy; use a no-cache build for this app if you rely on it")
	}
	if !app.PreviewActive {
		return nil, issues
	}
	if app.PreviewWildcard != "" {
		issues = appendIssue(issues, "previewWildcard", issueReview, fmt.Sprintf("preview wildcard host %q is not migrated; previews use the platform preview domain", app.PreviewWildcard))
	}
	if app.PreviewLimit < 1 {
		issues = appendIssue(issues, "previewLimit", issueReview, "preview deployments are on without a limit; the platform cap applies")
	}
	return &mappedPreviews{MaxPreviews: max(app.PreviewLimit, 0)}, issues
}

// Dokploy's docker (prebuilt image) and drop (file upload) sources have no image field to map, so they need manual review.
func mapDokployBuildConfig(app dokployApplication) (buildType, buildPath string, issue *migrationIssue, blocking bool) {
	switch app.SourceType {
	case "docker":
		return "", "", blockingIssue("sourceType", "Dokploy's docker (prebuilt image) source type has no app.yaml build path today; create this app manually with the target image"), true
	case "drop":
		return "", "", blockingIssue("sourceType", "Dokploy's drop (file upload) source type has no git or image source Levelrail can use; create this app manually"), true
	}

	switch app.BuildType {
	case "dockerfile":
		return spec.BuildDockerfile, cleanDokployPath(app.Dockerfile), nil, false
	case "railpack":
		return spec.BuildRailpack, "", nil, false
	case "static":
		return spec.BuildStatic, "", nil, false
	case "nixpacks":
		return "", "", blockingIssue("buildType", "Dokploy's nixpacks build type has no Levelrail equivalent (Levelrail uses Railpack, not Nixpacks); pick dockerfile, railpack, or static manually and create this app by hand"), true
	case "heroku_buildpacks", "paketo_buildpacks":
		return "", "", blockingIssue("buildType", fmt.Sprintf("Dokploy's %s build type has no Levelrail equivalent; pick dockerfile, railpack, or static manually and create this app by hand", app.BuildType)), true
	default:
		return "", "", blockingIssue("buildType", fmt.Sprintf("unrecognized Dokploy build_type %q", app.BuildType)), true
	}
}

func cleanDokployPath(loc string) string {
	loc = strings.TrimPrefix(strings.TrimSpace(loc), "/")
	if loc == "" || loc == "Dockerfile" {
		return ""
	}
	return loc
}

// Repo URLs are built only for unambiguous hosts; self-hosted GitLab and Gitea hosts are not in the API response.
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
			clean, out := sanitizeRepoForReport(app.CustomGitURL, out)
			return clean, app.CustomGitBranch, out
		}
	case "gitlab":
		if app.GitlabOwner != "" && app.GitlabRepository != "" {
			out = appendIssue(out, "repository", issueReview, fmt.Sprintf("Dokploy's API does not expose the GitLab instance host (often self-hosted); owner=%q repository=%q branch=%q, set the repo URL manually", app.GitlabOwner, app.GitlabRepository, app.GitlabBranch))
		}
	case "gitea":
		if app.GiteaOwner != "" && app.GiteaRepository != "" {
			out = appendIssue(out, "repository", issueReview, fmt.Sprintf("Dokploy's API does not expose the Gitea instance host (Gitea is always self-hosted); owner=%q repository=%q branch=%q, set the repo URL manually", app.GiteaOwner, app.GiteaRepository, app.GiteaBranch))
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
			issues = appendIssue(issues, "domains", issueDropped, fmt.Sprintf("domain port %d ignored: static sites have no running container to route to", port))
		}
		return hosts, 0, issues
	}

	for _, d := range domains[1:] {
		if d.Port != 0 && d.Port != port {
			issues = appendIssue(issues, "domains", issueReview, fmt.Sprintf("domain %q declares port %d, different from the first domain's port %d; app.yaml supports one port per service, only %d was migrated", d.Host, d.Port, port, port))
		}
	}
	return hosts, port, issues
}

func mapDokployMemory(raw string, issues []migrationIssue) (string, []migrationIssue) {
	bytes, issues := parseNumericLimit(raw, "memoryLimit", issues, func(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }, true,
		func(raw string, _ error) string {
			return fmt.Sprintf("could not parse memoryLimit %q as a byte count, resources.memory left unset", raw)
		})
	if bytes == 0 {
		return "", issues
	}
	return formatMemoryMiGi(bytes), issues
}

func mapDokployCPU(raw string, issues []migrationIssue) (float64, []migrationIssue) {
	nanocpus, issues := parseNumericLimit(raw, "cpuLimit", issues, func(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }, true,
		func(raw string, _ error) string {
			return fmt.Sprintf("could not parse cpuLimit %q as a nanoCPU count, resources.cpu left unset", raw)
		})
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

// Dokploy returns env values to any caller with app access, so includeSecretValues only gates writing them.
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
		issues = appendIssue(issues, "env", issueReview, fmt.Sprintf("%d env var(s) migrated as key-only secret placeholders; pass --include-secret-values to write the real values already returned by Dokploy into a companion secrets file or apply them via PUT .../secrets/{key}", len(keys)))
		return keys, nil, issues
	}
	return keys, parsed, issues
}

// discoverDokployCompose maps compose stacks through the read-only import source. A read failure
// only warns, so applications still migrate when the stacks cannot be read.
func discoverDokployCompose(ctx context.Context, f migrateDokployFlags, lookupEnv func(string) (string, bool), stderr io.Writer) []mappedApp {
	src, err := platformimport.NewDokploy(f.dokployURL, f.dokployToken, platformimport.ClientOptions{Policy: platformimport.NetworkPolicy{
		AllowPrivate: envIsTrue(lookupEnv, platformimport.AllowPrivateEnv), AllowLoopback: envIsTrue(lookupEnv, platformimport.AllowLoopbackEnv)}})
	if err == nil {
		var disc *platformimport.Discovery
		if disc, err = src.Discover(ctx); err == nil {
			out := make([]mappedApp, 0, len(disc.ComposeApps))
			for _, c := range disc.ComposeApps {
				out = append(out, mapDokployCompose(c, f.includeSecretValues))
			}
			return out
		}
	}
	_, _ = fmt.Fprintf(stderr, "warning: could not read Dokploy compose stacks, they are not included: %v\n", err)
	return nil
}

// mapDokployCompose stages a stack as a compose app. Env values are staged only when real values
// were requested, matching the env import, since a key name cannot prove a value is harmless.
func mapDokployCompose(c platformimport.ComposeApp, includeSecretValues bool) mappedApp {
	name, issues := mapServiceName("Dokploy", c.Name, nil)
	out := mappedApp{SourceName: c.Name, ServiceName: name}
	for _, n := range c.Notes {
		issues = appendIssue(issues, "compose", issueReview, n.Reason+", "+n.Manual)
	}
	if c.Verdict != platformimport.ComposeReady {
		for _, r := range c.Blocked {
			issues = appendIssue(issues, "compose", issueBlocking, r)
		}
		out.Issues, out.Blocking = issues, true
		return out
	}
	plain, secrets := c.SplitEnv()
	literal := map[string]string{}
	if includeSecretValues {
		for _, m := range []map[string]string{plain, secrets} {
			for k, v := range m {
				literal[k] = v
			}
		}
	} else if len(plain)+len(secrets) > 0 {
		issues = appendIssue(issues, "env", issueReview, fmt.Sprintf("%d env var(s) staged as names only; re-run with --include-secret-values to write their values into stack.env", len(plain)+len(secrets)))
	}
	keys := make([]string, 0, len(plain)+len(secrets))
	for k := range plain {
		keys = append(keys, k)
	}
	for k := range secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out.Service.EnvKeys, out.Service.Domains = keys, c.Domains
	if len(literal) > 0 {
		out.Service.EnvLiteral = literal
	}
	out.Compose = &stagedCompose{Yaml: c.Compose, Hosts: c.Domains, Volumes: c.Volumes}
	out.Issues = issues
	return out
}
