package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// runAppsDeploySpec implements "apps deploy-spec <name> --file app.yaml
// --repo-url <url> --ref <ref>": POST /api/v1/apps/{name}/deploy-spec
// (internal/api/apps_multi.go's handleDeploySpec), fanning every service
// app.yaml declares out into an independent build+deploy under one app
// named <name>. Unlike "apps create --file --service", which picks one
// service out of a multi-service file, this deploys every service the
// file declares in one call.
func runAppsDeploySpec(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "apps deploy-spec", "print the full deploy result as JSON to stdout and nothing else", stderr)
	var file, repoURL, ref, imageRepoBase string
	fs.StringVar(&file, "file", "", "path to an app.yaml with a services: map (required)")
	fs.StringVar(&repoURL, "repo-url", "", "git repository URL to build every service from (required)")
	fs.StringVar(&ref, "ref", "", "branch, tag, or commit to build (required)")
	fs.StringVar(&imageRepoBase, "image-repo-base", "", "image name prefix for every built service (defaults to <name>)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploySpecUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	name, ok := requireOneArg(fs, stderr, prog, "apps deploy-spec", "app name")
	if !ok {
		return exitUsage
	}

	if file == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--file is required"))
	}
	if repoURL == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--repo-url is required"))
	}
	if ref == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--ref is required"))
	}

	data, readErr := os.ReadFile(file) //nolint:gosec // operator-supplied CLI flag, same pattern apps_deploy_compose.go's own --file read uses
	if readErr != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("read %s: %w", file, readErr))
	}
	fileSpec, parseErr := spec.Parse(data)
	if parseErr != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("parse %s: %v", file, parseErr))
	}
	if len(fileSpec.Services) == 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s declares no services", file))
	}

	req := deploySpecRequest{
		RepoURL:       repoURL,
		Ref:           ref,
		ImageRepoBase: imageRepoBase,
		Services:      make(map[string]deploySpecService, len(fileSpec.Services)),
	}
	for key, svc := range fileSpec.Services {
		converted, convErr := toDeploySpecService(svc)
		if convErr != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("service %q: %v", key, convErr))
		}
		req.Services[key] = converted
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	result, err := client.DeploySpec(context.Background(), name, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("deploy spec to app %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, result); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printDeploySpecResultHuman(stdout, result)
	if !result.AllSucceeded {
		return exitAPIError
	}
	return exitOK
}

// toDeploySpecService converts one app.yaml service into deploy-spec's
// own wire shape (apiclient.DeploySpecService): unlike apps_create.go's
// toServiceResources/toServiceHealth, this keeps app.yaml's own
// human-readable strings (memory "512Mi", durations "5s") rather than
// converting to bytes/nanoseconds, because handleDeploySpec decodes
// straight into internal/spec.Service, not through appResource.
func toDeploySpecService(svc spec.Service) (deploySpecService, error) {
	if secretKeys := secretEnvKeys(svc.Env); len(secretKeys) > 0 {
		return deploySpecService{}, fmt.Errorf("declares secret env var(s) %v; apps deploy-spec does not yet set secrets", secretKeys)
	}

	out := deploySpecService{
		Build: deploySpecServiceBuild{
			Type:               svc.Build.Type,
			Path:               svc.Build.Path,
			BaseDirectory:      svc.Build.BaseDirectory,
			Image:              svc.Build.Image,
			RegistryCredential: svc.Build.RegistryCredential,
		},
		Domains:  svc.Domains,
		Port:     svc.Port,
		Replicas: svc.Replicas,
		Strategy: svc.Strategy,
	}
	if len(svc.Env) > 0 {
		out.Env = make(map[string]deploySpecServiceEnv, len(svc.Env))
		for k, v := range svc.Env {
			out.Env[k] = deploySpecServiceEnv{Value: v.Value, From: v.From}
		}
	}
	return out, nil
}

func appsDeploySpecUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploy-spec <name> --file app.yaml --repo-url <url> --ref <ref> [flags]

Parses an app.yaml with a services: map and deploys every declared
service as an independent build+deploy under app <name>.

Flags:
  --file string              path to an app.yaml with a services: map (required)
  --repo-url string          git repository URL to build every service from (required)
  --ref string                branch, tag, or commit to build (required)
  --image-repo-base string   image name prefix for every built service (defaults to <name>)
  --token string             API token (default: %[2]s env var, then the credentials file)
  --api-url string           control plane base URL (default: %[3]s env var, then %[4]s)
  --json                     print the full deploy result as JSON to stdout, nothing else
  -h, --help                 show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
