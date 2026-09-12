package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// runAppsDeploySpec deploys every service in app.yaml under one app, unlike
// "apps create --file --service" which picks a single service.
func runAppsDeploySpec(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploy-spec", "print the full deploy result as JSON to stdout and nothing else", stderr)
	var file, repoURL, ref, imageRepoBase string
	fs.StringVar(&file, "file", "", "path to an app.yaml with a services: map (required)")
	fs.StringVar(&repoURL, "repo-url", "", "git repository URL to build every service from (required)")
	fs.StringVar(&ref, "ref", "", "branch, tag, or commit to build (required)")
	fs.StringVar(&imageRepoBase, "image-repo-base", "", "image name prefix for every built service (defaults to <name>)")
	secretValues := make(map[string]string)
	fs.Var(stringMapFlag(secretValues), "secret", "secret env var value as KEY=VALUE, repeatable; applied to any service declaring that env var name as { secret: true }, same value-on-the-command-line convention \"apps secrets set\" already uses")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploySpecUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

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
		req.Services[key] = toDeploySpecService(svc)
	}
	applyDeploySpecSecrets(req.Services, secretValues)

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.DeploySpec(context.Background(), name, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("deploy spec to app %q: %w", name, err))
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

// toDeploySpecService keeps app.yaml's human-readable strings ("512Mi", "5s")
// as-is, unlike apps_create.go's converters which encode to bytes/nanoseconds.
func toDeploySpecService(svc spec.Service) deploySpecService {
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
			out.Env[k] = deploySpecServiceEnv{Value: v.Value, From: v.From, Secret: v.Secret, Required: v.Required}
		}
	}
	return out
}

// applyDeploySpecSecrets sets Value on every { secret: true } env entry
// services declares whose key has a matching value in secretValues:
// app.yaml itself can never carry a secret value (spec.EnvVar's own YAML
// shape has no value field alongside secret), so this is the only way a
// declared name gets a value as part of the same "apps deploy-spec" call,
// the multi-service equivalent of apps_create.go's --secret flag. A
// service key not declared { secret: true } is left untouched even if
// its own env var name happens to match: only names the app.yaml itself
// marked secret ever accept a --secret value.
func applyDeploySpecSecrets(services map[string]deploySpecService, secretValues map[string]string) {
	if len(secretValues) == 0 {
		return
	}
	for _, svc := range services {
		for envKey, v := range svc.Env {
			value, ok := secretValues[envKey]
			if !v.Secret || !ok {
				continue
			}
			v.Value = value
			svc.Env[envKey] = v // svc.Env is the same map services[key].Env points to
		}
	}
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
  --secret KEY=VALUE         secret env var value, repeatable; applied to any service declaring
                               that name as { secret: true }, stored via envelope-encrypted
                               secret storage as part of this same deploy call
  --token string             API token (default: %[2]s env var, then the credentials file)
  --api-url string           control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string           named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the full deploy result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
