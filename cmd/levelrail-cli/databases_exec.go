package main

import (
	"context"
	"fmt"
	"io"
)

// runDatabasesExec implements "databases exec <name> -- <command>
// [args...]": POST /api/v1/databases/{name}/exec
// (internal/api/database_exec.go's own handleExecDatabase). Mirrors
// runAppsExec's shape exactly, including the "--" separator requirement
// and the real-remote-exit-code-as-process-exit-code behavior: see that
// function's own doc comment for the full reasoning, unchanged here.
// The one thing this makes possible that apps exec doesn't: running an
// engine's own client inside its container, e.g.
// "levelrail-cli databases exec main -- redis-cli ping" or
// "levelrail-cli databases exec main -- psql -U postgres -c '\\l'".
func runDatabasesExec(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases exec", "print the full exec result (stdout, stderr, exit_code, truncated) as JSON to stdout and nothing else, instead of writing stdout/stderr directly", stderr)
	var timeoutSeconds int
	fs.IntVar(&timeoutSeconds, "timeout", defaultExecTimeoutSeconds, "command timeout in seconds (the server enforces a hard ceiling of its own; this can only ask for less, never more)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesExecUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if len(rest) < 2 {
		_, _ = fmt.Fprintf(stderr, "%s: databases exec requires a database name and a command, e.g. \"%s databases exec main -- redis-cli ping\"\n\n", prog, prog)
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

	result, err := client.ExecDatabase(context.Background(), name, execRequest{Command: command, Args: cmdArgs, TimeoutSeconds: timeoutSeconds})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("exec in database %q: %w", name, err))
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

func databasesExecUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases exec <name> -- <command> [args...] [flags]

Runs command inside database <name>'s currently running container and
waits for it to finish (not an interactive shell: no PTY, no resize,
nothing kept alive between calls). Prints the command's real
stdout/stderr and exits this process with the command's own real exit
code, not a generic CLI success/failure code, so a caller can script
against it directly ("%[1]s databases exec main -- redis-cli ping").

The "--" before the command is required whenever the command itself
takes flags (anything starting with "-"): without it, this CLI's own
flag parser will try to consume that flag as its own and reject it as
unknown. Always safe to include, so the example above does.

Flags:
  --timeout int            command timeout in seconds, default %[5]d (the server enforces its own hard ceiling; this can only ask for less)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the full exec result (stdout, stderr, exit_code, truncated) as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, defaultExecTimeoutSeconds)
}
