package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runAppsPreviewsExtend(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps previews extend", "print the preview as JSON to stdout and nothing else", stderr)
	hours := fs.Int("hours", 24, "hours to add to the preview's expiry")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsPreviewsExtendUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	positional := fs.Args()
	if len(positional) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps previews extend requires an app name and a pr number\n\n", prog)
		_, _ = fmt.Fprint(stderr, appsPreviewsExtendUsage(prog))
		return exitUsage
	}
	appName := positional[0]
	prNumber, err := strconv.Atoi(positional[1])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("pr number must be an integer"))
	}
	if *hours < 1 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--hours must be at least 1"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	p, err := client.ExtendPreviewEnvironment(context.Background(), appName, prNumber, *hours)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("extend preview for app %q pr #%d: %w", appName, prNumber, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, p, func() {
		_, _ = fmt.Fprintf(stdout, "preview for app %q pr #%d now expires at %s\n", appName, prNumber, p.ExpiresAt)
	})
}

func appsPreviewsExtendUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps previews extend <app-name> <pr-number> [--hours 24] [flags]

Pushes one preview's expiry out by the given hours, from whichever is
later: now or its current expiry. The preview is still removed when its
pull request closes.

Flags:
  --hours int             hours to add (default 24)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsPreviewsSettings(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps previews settings", "print the settings as JSON to stdout and nothing else", stderr)
	onLimit := fs.String("on-limit", "", "when the cap is full: evict_oldest or reject")
	allowForks := fs.Bool("allow-forks", false, "deploy pull requests from forks without approval (they still get no environment unless --fork-secrets)")
	forkSecrets := fs.Bool("fork-secrets", false, "give fork previews the app's environment variables and secrets")
	ttlHours := fs.Int("ttl-hours", 0, "hours a preview lives without updates (0 uses the platform default)")
	maxPreviews := fs.Int("max-previews", 0, "most live previews for this app (0 uses the platform cap)")
	memory := fs.String("memory", "", "memory limit per preview, for example 256Mi (empty uses the platform default)")
	cpu := fs.Float64("cpu", 0, "cpu limit per preview (0 uses the platform default)")
	idle := fs.Int("idle-sleep-minutes", 0, "sleep a preview after this many idle minutes (0 uses the platform default)")
	dbStrategy := fs.String("database", "", "database strategy: none, shared, fresh or seed")
	seed := fs.String("seed-database", "", "database whose latest backup seeds a preview (seed strategy)")
	gate := fs.Bool("gate", false, "require basic auth in front of every preview (set --gate-username and --gate-password)")
	gateUser := fs.String("gate-username", "", "basic auth username for previews")
	gatePass := fs.String("gate-password", "", "basic auth password for previews (write-only)")
	indexing := fs.Bool("allow-indexing", false, "let search engines index preview URLs (hidden by default)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsPreviewsSettingsUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	appName, ok := requireOneArg(fs, stderr, prog, "apps previews settings", "app name")
	if !ok {
		return exitUsage
	}

	var req apiclient.SetPreviewPolicyRequest
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "on-limit":
			req.OnLimit = onLimit
		case "allow-forks":
			req.AllowForkPreviews = allowForks
		case "fork-secrets":
			req.AllowForkSecrets = forkSecrets
		case "ttl-hours":
			req.TTLHours = ttlHours
		case "max-previews":
			req.MaxPreviews = maxPreviews
		case "memory":
			req.MemoryLimit = memory
		case "cpu":
			req.CPULimit = cpu
		case "idle-sleep-minutes":
			req.IdleSleepMinutes = idle
		case "database":
			req.DatabaseStrategy = dbStrategy
		case "seed-database":
			req.SeedDatabase = seed
		case "gate":
			req.GateBasicAuth = gate
		case "gate-username":
			req.GateUsername = gateUser
		case "gate-password":
			req.GatePassword = gatePass
		case "allow-indexing":
			req.AllowIndexing = indexing
		}
	})

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var (
		policy apiclient.PreviewPolicyResource
		err    error
	)
	if req != (apiclient.SetPreviewPolicyRequest{}) {
		policy, err = client.SetPreviewPolicy(ctx, appName, req)
	} else {
		policy, err = client.GetPreviewPolicy(ctx, appName)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("preview settings for app %q: %w", appName, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, policy, func() { printPreviewSettings(stdout, appName, policy) })
}

func printPreviewSettings(out io.Writer, appName string, p apiclient.PreviewPolicyResource) {
	yes := map[bool]string{true: "yes", false: "no"}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "app\t%s\n", appName)
	_, _ = fmt.Fprintf(tw, "live previews (this app)\t%d of %s\n", p.LiveCount, limitLabel(p.MaxPerApp))
	_, _ = fmt.Fprintf(tw, "when the limit is full\t%s\n", p.OnLimit)
	_, _ = fmt.Fprintf(tw, "ttl\t%d hours\n", p.EffectiveTTLHours)
	_, _ = fmt.Fprintf(tw, "memory, cpu\t%s, %g\n", p.EffectiveMemory, p.EffectiveCPU)
	_, _ = fmt.Fprintf(tw, "idle sleep\t%d minutes\n", p.EffectiveIdleSleepMinutes)
	_, _ = fmt.Fprintf(tw, "database strategy\t%s\n", p.DatabaseStrategy)
	if p.SeedDatabase != "" {
		_, _ = fmt.Fprintf(tw, "seed database\t%s\n", p.SeedDatabase)
	}
	_, _ = fmt.Fprintf(tw, "fork pull requests\t%s\n", map[bool]string{true: "deploy automatically", false: "wait for approval"}[p.AllowForkPreviews])
	_, _ = fmt.Fprintf(tw, "fork previews get secrets\t%s\n", yes[p.AllowForkSecrets])
	_, _ = fmt.Fprintf(tw, "basic auth gate\t%s\n", yes[p.GateBasicAuth])
	_, _ = fmt.Fprintf(tw, "indexed by search engines\t%s\n", yes[p.AllowIndexing])
	_ = tw.Flush()
}

func appsPreviewsSettingsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps previews settings <app-name> [flags]

Shows one app's preview policy, or changes it when any policy flag is
given. Safe defaults: no production database, no secrets for forks, hidden
from search engines, smaller resources, sleep when idle.

Flags:
  --on-limit string          when the cap is full: evict_oldest (default) or reject
  --max-previews int         most live previews for this app (0 uses the platform cap)
  --ttl-hours int            hours a preview lives without updates (0 uses the platform default)
  --memory string            memory limit per preview, for example 256Mi
  --cpu float                cpu limit per preview
  --idle-sleep-minutes int   sleep a preview after this many idle minutes
  --database string          database strategy: none (default), shared, fresh or seed
  --seed-database string     database whose latest backup seeds a preview
  --allow-forks              deploy fork pull requests without approval
  --fork-secrets             give fork previews the app's environment and secrets
  --gate                     require basic auth on previews (needs --gate-username and --gate-password)
  --gate-username string     basic auth username
  --gate-password string     basic auth password (write-only)
  --allow-indexing           let search engines index preview URLs
  --token string             API token (default: %[2]s env var, then the credentials file)
  --api-url string          control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string          named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                       print the result as JSON to stdout, nothing else
  --output string             output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string              JMESPath expression to filter the result before printing
  -h, --help                  show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
