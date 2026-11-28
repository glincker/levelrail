package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/glincker/levelrail/internal/apiclient"
)

// runRegistryCredentialsHub serves "status" and "limits": both read a stored Docker Hub token's health.
func runRegistryCredentialsHub(prog, sub string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	name := "registry-credentials " + sub
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, name, "print the result as JSON to stdout and nothing else", stderr)
	var refresh bool
	fs.BoolVar(&refresh, "refresh", false, "check Docker Hub now instead of reusing a recent reading")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, registryCredentialsHubUsage(prog, sub)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, name, "registry credential id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	var result any
	var render func()
	if sub == "limits" {
		lim, err := client.GetDockerHubLimits(context.Background(), id, refresh)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("read Docker Hub pull limits for %q: %w", id, err))
		}
		result, render = lim, func() { printDockerHubLimits(stdout, &lim) }
	} else {
		st, err := client.GetDockerHubStatus(context.Background(), id, refresh)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("read Docker Hub status for %q: %w", id, err))
		}
		result, render = st, func() { printDockerHubStatus(stdout, st) }
	}
	if err := renderResult(stdout, of.Format, of.Query, result, render); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printDockerHubLimits(out io.Writer, l *apiclient.DockerHubLimitsResource) {
	who := "anonymous"
	if l.Authenticated {
		who = "authenticated"
	}
	if !l.Known {
		_, _ = fmt.Fprintf(out, "pull limit: not reported (%s)\n", who)
		return
	}
	_, _ = fmt.Fprintf(out, "pull limit: %d of %d remaining per %ds (%s, source %s)\n", l.Remaining, l.Limit, l.WindowSeconds, who, l.Source)
	if l.Low {
		_, _ = fmt.Fprintf(out, "warning: remaining pulls are below %d%% of the limit\n", l.WarnPercent)
	}
}

func printDockerHubStatus(out io.Writer, s apiclient.DockerHubStatusResource) {
	if !s.Valid {
		_, _ = fmt.Fprintf(out, "token: invalid (%s)\n", s.Message)
		return
	}
	_, _ = fmt.Fprintf(out, "token: valid (%s, namespace %s), checked %s\n", s.Kind, s.Namespace, s.CheckedAt.Format("2006-01-02 15:04:05Z07:00"))
	if len(s.Scopes) > 0 {
		_, _ = fmt.Fprintf(out, "scopes: %s\n", strings.Join(s.Scopes, ", "))
	}
	for _, c := range s.Capabilities {
		_, _ = fmt.Fprintf(out, "  %s: %s\n", c.Name, c.Status)
	}
	if s.ExpiryKnown && s.ExpiresAt != nil {
		_, _ = fmt.Fprintf(out, "expires: %s\n", s.ExpiresAt.Format("2006-01-02"))
	}
	if s.Limits != nil {
		printDockerHubLimits(out, s.Limits)
	}
}

func registryCredentialsHubUsage(prog, sub string) string {
	what := "Checks the stored Docker Hub token and shows its scopes, last check time and pull limit."
	if sub == "limits" {
		what = "Shows the Docker Hub pull limit (limit, remaining, window, source) the stored token gets."
	}
	return fmt.Sprintf(`Usage:
  %[1]s registry-credentials %[2]s <id> [flags]

%[3]s

Flags:
  --refresh              check Docker Hub now instead of reusing a recent reading
  --token string         API token (default: %[4]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[5]s env var, then %[6]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                 print the result as JSON to stdout, nothing else
  --output string        output format: json, ndjson, yaml, table, wide, text or name
  --query string         JMESPath expression to filter the result before printing
  -h, --help             show this help
`, prog, sub, what, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
