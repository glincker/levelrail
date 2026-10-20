package main

import (
	"fmt"
	"sort"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// Pure, no I/O. CapRover exposes no repo or branch, so RepoURL and Ref stay empty.
func mapCaproverApplication(app caproverAppDefinition, rootDomain string, includeSecretValues bool) mappedApp {
	out := mappedApp{SourceName: app.AppName}

	out.ServiceName, out.Issues = mapServiceName("CapRover", app.AppName, out.Issues)

	out.Service.BuildType, out.Issues = mapCaproverBuildConfig(app, out.Issues)
	out.Service.Domains = mapCaproverDomains(app, rootDomain)
	out.Service.Port = app.ContainerHTTPPort
	out.Issues = mapCaproverPorts(app.Ports, out.Issues)
	out.Issues = mapCaproverVolumes(app, out.Issues)
	out.Issues = mapCaproverInstanceCount(app.InstanceCount, out.Issues)
	out.Service.EnvKeys, out.Service.EnvLiteral, out.Issues = mapCaproverEnv(app.EnvVars, includeSecretValues, out.Issues)

	return out
}

// The build method isn't exposed by the listing, so this always assumes a root Dockerfile.
func mapCaproverBuildConfig(app caproverAppDefinition, issues []migrationIssue) (string, []migrationIssue) {
	path := app.CaptainDefinitionRelativeFilePath
	if path == "" {
		path = "./captain-definition"
	}
	issues = appendIssue(issues, "captainDefinitionRelativeFilePath", issueReview, fmt.Sprintf(
		"CapRover's %s can declare a Dockerfile path, a template ID, or inline Dockerfile lines; this API does not expose its contents, so build.type was assumed to be dockerfile at the repo root, confirm and adjust before deploying",
		path))
	return spec.BuildDockerfile, issues
}

// Includes the default subdomain plus every custom domain, since app.yaml takes a list.
func mapCaproverDomains(app caproverAppDefinition, rootDomain string) []string {
	var domains []string
	if !app.NotExposeAsWebApp && rootDomain != "" {
		domains = append(domains, app.AppName+"."+rootDomain)
	}
	for _, d := range app.CustomDomain {
		if d.PublicDomain != "" {
			domains = append(domains, d.PublicDomain)
		}
	}
	return domains
}

// mapCaproverPorts reports CapRover's raw host<->container TCP/UDP port
// mappings as dropped: mappedService carries only ContainerHTTPPort's
// single ingress-routed port, no field for additional raw port bindings.
func mapCaproverPorts(ports []caproverPort, issues []migrationIssue) []migrationIssue {
	for _, p := range ports {
		issues = appendIssue(issues, "ports", issueDropped, fmt.Sprintf(
			"host port %d -> container port %d has no app.yaml equivalent (app.yaml supports one ingress-routed port per service) and was dropped",
			p.HostPort, p.ContainerPort))
	}
	return issues
}

// mapCaproverVolumes reports persistent volumes as dropped: app.yaml has
// no service-level volume field, so a CapRover app with persistent data
// needs its storage recreated manually before this app is deployed.
func mapCaproverVolumes(app caproverAppDefinition, issues []migrationIssue) []migrationIssue {
	for _, v := range app.Volumes {
		detail := fmt.Sprintf("persistent volume %s -> %s has no app.yaml equivalent and was dropped; recreate this storage manually before deploying", v.VolumeName, v.ContainerPath)
		if v.HostPath != "" {
			detail = fmt.Sprintf("bind mount %s -> %s has no app.yaml equivalent and was dropped; recreate this storage manually before deploying", v.HostPath, v.ContainerPath)
		}
		issues = appendIssue(issues, "volumes", issueDropped, detail)
	}
	if app.HasPersistentData && len(app.Volumes) == 0 {
		issues = appendIssue(issues, "volumes", issueReview, "hasPersistentData is set but no volumes were listed; check the CapRover dashboard for this app's persistent storage before deploying")
	}
	return issues
}

// mappedService has no replica field, so the generated app.yaml uses the default count.
func mapCaproverInstanceCount(instanceCount int, issues []migrationIssue) []migrationIssue {
	if instanceCount > 1 {
		issues = appendIssue(issues, "instanceCount", issueReview, fmt.Sprintf(
			"CapRover instanceCount is %d; this migration tool does not set app.yaml's replicas field, set it manually if you need more than one instance", instanceCount))
	}
	return issues
}

// mapCaproverEnv always returns every CapRover env var's key. CapRover's
// appDefinitions listing has no documented redaction ability gate, the
// same trust model mapDokployEnv already documents for Dokploy's env blob.
func mapCaproverEnv(envVars []caproverEnvVar, includeSecretValues bool, issues []migrationIssue) ([]string, map[string]string, []migrationIssue) {
	if len(envVars) == 0 {
		return nil, nil, issues
	}

	keys := make([]string, 0, len(envVars))
	for _, e := range envVars {
		keys = append(keys, e.Key)
	}
	sort.Strings(keys)

	if !includeSecretValues {
		issues = appendIssue(issues, "envVars", issueReview, fmt.Sprintf("%d env var(s) migrated as key-only secret placeholders; pass --include-secret-values to write the real values already returned by CapRover into a companion secrets file or apply them via PUT .../secrets/{key}", len(keys)))
		return keys, nil, issues
	}

	literal := make(map[string]string, len(envVars))
	for _, e := range envVars {
		literal[e.Key] = e.Value
	}
	return keys, literal, issues
}
