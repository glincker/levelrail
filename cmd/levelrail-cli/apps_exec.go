package main

import (
	"context"
	"fmt"
	"io"
)

// defaultExecTimeoutSeconds mirrors the server's default exec timeout. It is only a flag default; the server enforces the real ceiling.
const defaultExecTimeoutSeconds = 30

// runAppsExec implements "apps exec <name> -- <command>". The "--" is required so remote args starting with "-" are not parsed as flags.
// On success it returns the remote command's own exit code rather than the CLI's exit enum.
func runAppsExec(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps exec", "print the full exec result (stdout, stderr, exit_code, truncated) as JSON to stdout and nothing else, instead of writing stdout/stderr directly", stderr)
	var timeoutSeconds int
	fs.IntVar(&timeoutSeconds, "timeout", defaultExecTimeoutSeconds, "command timeout in seconds (the server enforces a hard ceiling of its own; this can only ask for less, never more)")
	var interactive bool
	fs.BoolVar(&interactive, "interactive", false, "open a real interactive terminal (PTY, resize, a shell kept alive) instead of running one command and printing its output")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsExecUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if interactive {
		return runAppsExecInteractiveFromFlags(fs.Args(), prog, credentialFlags{Token: tokenFlag, APIURL: apiURLFlag, Profile: profileFlag}, lookupEnv, stdout, stderr)
	}
	if len(rest) < 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps exec requires an app name and a command, e.g. \"%s apps exec web -- ls -la /\"\n\n", prog, prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]
	command := rest[1]
	cmdArgs := rest[2:]

	if timeoutSeconds < 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--timeout must not be negative"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.ExecApp(context.Background(), name, execRequest{Command: command, Args: cmdArgs, TimeoutSeconds: timeoutSeconds})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("exec in app %q: %w", name, err))
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

func appsExecUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps exec <name> -- <command> [args...] [flags]
  %[1]s apps exec <name> --interactive [-- <shell> [args...]]

Runs command inside app <name>'s currently running container and waits
for it to finish. Prints the command's real stdout/stderr and exits this
process with the command's own real exit code, not a generic CLI
success/failure code, so a caller can script against it directly
("%[1]s apps exec web -- test -f /ready.lock && echo ready").

With --interactive you get a real terminal instead: a PTY, a shell kept
alive, arrow keys and Ctrl-C working, and the remote terminal resizing
with this one. It needs a terminal on stdin, and names no command by
default (the server starts bash where the image has it, sh otherwise).

The "--" before the command is required whenever the command itself
takes flags (anything starting with "-"): without it, this CLI's own
flag parser will try to consume that flag as its own and reject it as
unknown. Always safe to include, so every example here does.

Flags:
  --interactive            open a real interactive terminal instead of running one command
  --timeout int            command timeout in seconds, default %[5]d (the server enforces its own hard ceiling; this can only ask for less, and it does not apply to --interactive)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the full exec result (stdout, stderr, exit_code, truncated) as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, defaultExecTimeoutSeconds)
}
