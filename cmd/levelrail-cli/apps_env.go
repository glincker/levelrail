package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// defaultEnvPullFile mirrors "vercel env pull"'s own default filename.
const defaultEnvPullFile = ".env"

// runAppsEnv dispatches "apps env <verb> [flags]" to "pull", the only
// verb so far. An app's own env vars have no existing CLI read/write
// pair the way projects/organizations/environments do (those go through
// PUT /api/v1/apps/{name} as a whole-app update instead), so this is a
// new sub-namespace rather than an addition to an existing flat one.
func runAppsEnv(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsEnvUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsEnvUsage(prog))
		return exitOK
	case "pull":
		return runAppsEnvPull(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps env subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsEnvUsage(prog))
		return exitUsage
	}
}

func appsEnvUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps env pull <name> [file] [flags]   write an app's env vars to a local .env file

Run "%[1]s apps env <subcommand> -h" for a subcommand's own flags.
`, prog)
}

// runAppsEnvPull implements "apps env pull <name> [file]": the inverse
// of the dashboard's paste-a-.env-block import (EnvVarsForm's
// parseEnvBlock). Reads GET /api/v1/apps/{name} for the app's plain env
// (appResource.Env) and GET /api/v1/apps/{name}/secrets for its secret
// keys, and writes both to file in .env format. Secret values are never
// returned by any read endpoint, so each secret key gets a commented
// explanation instead of a real value.
func runAppsEnvPull(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, _, _, _ := apiFlagSet(prog, "apps env pull", "unused for this subcommand", stderr)
	var force bool
	fs.BoolVar(&force, "force", false, "overwrite the file if it already exists")
	fs.BoolVar(&force, "f", false, "shorthand for --force")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps env pull <name> [file] [flags]\n\n", prog)
		_, _ = fmt.Fprintln(stderr, appsEnvPullDescription)
		_, _ = fmt.Fprintln(stderr, "\nFlags:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag := *tokenFlagP, *apiURLFlagP, *profileFlagP

	rest := fs.Args()
	if len(rest) != 1 && len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps env pull requires an app name and an optional file path\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]
	file := defaultEnvPullFile
	if len(rest) == 2 {
		file = rest[1]
	}

	if !force {
		if _, err := os.Stat(file); err == nil {
			_, _ = fmt.Fprintf(stderr, "%s: %s already exists, use --force to overwrite\n", prog, file)
			return exitUsage
		} else if !os.IsNotExist(err) {
			_, _ = fmt.Fprintf(stderr, "%s: check %s: %v\n", prog, file, err)
			return exitUsage
		}
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	app, err := client.GetApp(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("get app %q: %w", name, err))
	}
	secrets, err := client.ListSecrets(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("list secrets for app %q: %w", name, err))
	}

	content := renderEnvFile(prog, name, app.Env, secrets)
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("write %s: %w", file, err))
	}

	_, _ = fmt.Fprintf(stdout, "wrote %d env var(s) and %d secret placeholder(s) to %s\n", len(app.Env), len(secrets), file)
	return exitOK
}

const appsEnvPullDescription = `Fetches an app's current plain env vars and secret keys and writes them
to file in .env format, defaulting to ".env" in the current directory.
Secret values are never returned by the API: each secret key is written
as a commented-out placeholder explaining it must be set manually.

Refuses to overwrite an existing file unless --force (or -f) is given.`

// renderEnvFile builds a .env-format file body from an app's plain env
// vars and secret keys, sorted so output is deterministic. A secret's
// line is commented out rather than written as a live "KEY=" (empty
// value): an empty value would misrepresent an unset secret as one
// deliberately set to the empty string, which matters to any tool that
// later reads this file back (docker compose --env-file, a shell
// source).
func renderEnvFile(prog, appName string, env map[string]string, secrets []secretKeyResource) string {
	var b strings.Builder

	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, formatEnvValue(env[k]))
	}

	if len(secrets) == 0 {
		return b.String()
	}
	if len(keys) > 0 {
		b.WriteString("\n")
	}

	sorted := make([]secretKeyResource, len(secrets))
	copy(sorted, secrets)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Key < sorted[j].Key })
	for _, s := range sorted {
		fmt.Fprintf(&b, "# %s is a secret; its value is never returned by the API.\n", s.Key)
		fmt.Fprintf(&b, "# Set it with: %s apps secrets set %s %s <value>\n", prog, appName, s.Key)
		fmt.Fprintf(&b, "# %s=\n", s.Key)
	}
	return b.String()
}

// formatEnvValue quotes v when it contains characters a .env parser
// would otherwise treat specially, matching what web/src/lib/envParse.ts's
// parseEnvBlock expects to read back: unquoted values are trimmed and a
// value wrapped in matching quotes has them stripped, so anything with
// leading/trailing space, a "#" that would start a comment, or a quote
// character of its own needs quoting to round-trip.
func formatEnvValue(v string) string {
	if !needsEnvQuoting(v) {
		return v
	}
	if !strings.Contains(v, `"`) {
		return `"` + v + `"`
	}
	if !strings.Contains(v, "'") {
		return "'" + v + "'"
	}
	return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
}

func needsEnvQuoting(v string) bool {
	if v == "" {
		return false
	}
	if strings.TrimSpace(v) != v {
		return true
	}
	return strings.ContainsAny(v, " \t\n#\"'\\$")
}
