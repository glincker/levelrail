package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsClone implements "apps clone <name> <new-name>": POST
// /api/v1/apps/{name}/clone. Domains, secret values, and node placement
// never carry over to the clone (internal/api/apps_clone.go's own doc
// comment); the caller re-sets those explicitly afterward.
func runAppsClone(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps clone", "print the cloned app as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsCloneUsage(prog)) }

	var copySecrets, preview bool
	var domainSuffix, envID string
	fs.BoolVar(&copySecrets, "copy-secrets", false, "copy secret values (re-encrypted for the new app; needs read:sensitive)")
	fs.StringVar(&domainSuffix, "domain-suffix", "", "derive domains by adding -SUFFIX to each source domain's first label (default: no domains)")
	fs.StringVar(&envID, "environment", "", "environment ID in the source's project to place the clone in")
	fs.BoolVar(&preview, "preview", false, "show what would and would not be copied, change nothing")

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	if preview {
		if len(fs.Args()) < 1 {
			_, _ = fmt.Fprintf(stderr, "%s: apps clone --preview requires an app name\n", prog)
			return exitUsage
		}
		client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
		prev, err := client.PreviewClone(context.Background(), fs.Args()[0])
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("preview clone of %q: %w", fs.Args()[0], err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, prev, func() {
			_, _ = fmt.Fprintf(stdout, "will copy:\n  %s\nwill not copy:\n  %s\n", strings.Join(prev.WillCopy, "\n  "), strings.Join(prev.WillNotCopy, "\n  "))
		})
	}

	rest, ok := requireArgs(fs, stderr, prog, "apps clone", "an app name and a new name", 2)
	if !ok {
		return exitUsage
	}
	name, newName := rest[0], rest[1]

	req := apiclient.CloneAppRequest{NewName: newName, CopySecrets: copySecrets, EnvironmentID: envID}
	if domainSuffix != "" {
		req.Domains, req.DomainSuffix = "suffix", domainSuffix
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	cloned, err := client.CloneAppWith(context.Background(), name, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clone app %q to %q: %w", name, newName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, cloned, func() { printAppHuman(stdout, cloned) })
}

func appsCloneUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps clone <name> <new-name> [flags]

Duplicates <name>'s desired state (image, port, env, secret names,
resources, health checks, strategy, replicas, hooks, volume definitions,
project) under <new-name>. Domains are left empty unless --domain-suffix
derives new ones; secret values are copied only with --copy-secrets;
volume data is never copied (volumes start empty); the clone starts on the
local node. Fails with a conflict if <new-name> already exists.

Flags:
  --copy-secrets            copy secret values, re-encrypted for the new app (needs read:sensitive)
  --domain-suffix string    derive domains by adding -SUFFIX to each first label
  --environment string      environment ID (of the source's project) for the clone
  --preview                 show what would and would not be copied
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the cloned app as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
