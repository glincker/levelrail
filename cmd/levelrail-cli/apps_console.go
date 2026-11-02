package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// consoleCommandFor resolves the console argv the app's template declares.
// The argv holds no secrets: the image reads its own from the container env.
func consoleCommandFor(ctx context.Context, client *Client, name string) ([]string, error) {
	app, err := client.GetApp(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get app %q: %w", name, err)
	}
	none := newValidationError("app %q has no console: its template declares no console command (use \"apps exec\" to run a command in it)", name)
	if app.TemplateID == "" {
		return nil, none
	}
	tpl, err := client.GetServiceTemplate(ctx, app.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("get template %q: %w", app.TemplateID, err)
	}
	if tpl.Manifest == nil || len(tpl.Manifest.ConsoleCommand) == 0 {
		return nil, none
	}
	return tpl.Manifest.ConsoleCommand, nil
}

// runAppsConsole implements "apps console <name> [-- command...]" on top of the exec paths: a terminal without a command, one-shot exec with one.
func runAppsConsole(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps console", "print the one-shot result (stdout, stderr, exit_code, truncated) as JSON to stdout and nothing else", stderr)
	var timeoutSeconds int
	fs.IntVar(&timeoutSeconds, "timeout", defaultExecTimeoutSeconds, "one-shot command timeout in seconds (the server enforces its own ceiling)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsConsoleUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) < 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps console requires an app name, e.g. \"%s apps console minecraft\"\n\n", prog, prog)
		fs.Usage()
		return exitUsage
	}
	if timeoutSeconds < 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--timeout must not be negative"))
	}
	name, extra := rest[0], rest[1:]

	creds := credentialFlags{Token: tokenFlag, APIURL: apiURLFlag, Profile: profileFlag}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	base, err := consoleCommandFor(context.Background(), client, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	if len(extra) == 0 {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			_, _ = fmt.Fprintf(stderr, "%s: apps console needs a terminal on stdin; pass the command after \"--\" for a one-shot, e.g. \"%s apps console %s -- list\"\n", prog, prog, name)
			return exitUsage
		}
		profile := resolveProfile(creds.Profile, lookupEnv)
		return runAppsExecInteractive(prog, name, base, terminalCredentials{
			APIURL: resolveAPIURL(creds.APIURL, lookupEnv, prog, profile),
			Token:  resolveToken(creds.Token, lookupEnv, prog, profile),
		}, stdout, stderr)
	}

	argv := append(append([]string{}, base[1:]...), extra...)
	result, err := client.ExecApp(context.Background(), name, execRequest{Command: base[0], Args: argv, TimeoutSeconds: timeoutSeconds})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("console in app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, result, func() {
		_, _ = io.WriteString(stdout, result.Stdout)
		if result.Stderr != "" {
			_, _ = io.WriteString(stderr, result.Stderr)
		}
		if result.Truncated {
			_, _ = fmt.Fprintf(stderr, "%s: output truncated (exceeded the server's per-response cap)\n", prog)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return result.ExitCode
}

func appsConsoleUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps console <name> [flags]
  %[1]s apps console <name> [flags] -- <command> [args...]

Opens the console the app's template declares (for Minecraft, rcon-cli),
using the same exec path as "apps exec". With no command you get a real
terminal; with one, it runs once, prints its output and exits with the
remote exit code. The console reads its credentials from the container's
own environment, so nothing secret is passed on a command line.

Apps whose template declares no console are refused; use "%[1]s apps exec"
to run a command in them.

Flags:
  --timeout int            one-shot timeout in seconds, default %[5]d (not used for the terminal)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string         control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string         named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                   print the one-shot result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL, defaultExecTimeoutSeconds)
}
