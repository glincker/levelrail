package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// defaultExecTimeoutSeconds mirrors internal/api/exec.go's own
// defaultExecTimeout (30 * time.Second): a hard server-side ceiling a
// caller may only lower via execRequest.TimeoutSeconds, never raise (see
// that constant's own doc comment). Used here only as this flag's
// default value, not enforced client-side: asking for --timeout 300
// sends 300 to the server and gets a real 200 back after 30s regardless,
// the same "the server is the actual authority on this" boundary every
// other wire-shape constant redeclared in this package already respects
// (client.go's own doc comment on why these types are redeclared, not
// imported).
const defaultExecTimeoutSeconds = 30

// runAppsExec implements "apps exec <name> -- <command> [args...]":
// POST /api/v1/apps/{name}/exec (internal/api/exec.go's own
// handleExecApp). AbilityRoot-gated server-side (see that file's own
// doc comment on why: secrets are injected as plaintext env vars, and
// exec can read them back out via `env`, the same tier node management
// sits at).
//
// The "--" before the remote command is not optional decoration: this
// command reuses reorderArgsFlagsFirst the same way every other
// name-first command in this package does, and that helper's own doc
// comment already establishes the limit that comes with it (Parse stops
// consuming flags at the first non-flag token). Without "--", a remote
// command argument that itself starts with "-" (an "ls -la" style flag
// meant for the container, not this CLI) will be mis-parsed as a flag
// of this command's own and rejected as unknown; docker exec and
// kubectl exec require the exact same "--" separator for the exact same
// reason. "apps exec web echo hello" (no dashed argument) happens to
// work without it, but writing "--" is always safe and is what every
// usage example below does.
//
// Exit code is deliberately not this CLI's usual exitOK/exitUsage/
// exitValidation/exitNetwork/exitAPIError enum on the success path: it
// is the remote command's own real exit code, echoed straight through
// (0 on the remote command's own success, whatever nonzero value it
// chose to fail with otherwise), because that real exit code, not a
// generic "the CLI call worked," is the entire reason a command shaped
// like "docker exec" exists at all: a caller scripts against it
// directly. The CLI's own exit codes are still used, unchanged, for
// every failure that happens before a remote exit code exists at all
// (bad flags, unreachable server, a 4xx/5xx from the API itself, a
// timeout that means the remote command's real exit code was never
// learned). Like every other command in this package, this function
// only ever returns an int; main() is the sole os.Exit call site
// (main.go's own doc comment), so table-driving this exact return value
// through run() is already how apps_exec_test.go verifies it, the same
// "does the work" vs "is the entrypoint" split cmd/levelrail's own
// recover_admin.go/recoverAdminUser establishes there, already present
// project-wide in this package's run()/main() split rather than
// needing a second one just for this command.
func runAppsExec(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps exec", "print the full exec result (stdout, stderr, exit_code, truncated) as JSON to stdout and nothing else, instead of writing stdout/stderr directly", stderr)
	var timeoutSeconds int
	fs.IntVar(&timeoutSeconds, "timeout", defaultExecTimeoutSeconds, "command timeout in seconds (the server enforces a hard ceiling of its own; this can only ask for less, never more)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsExecUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	rest := fs.Args()
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

	if jsonOut {
		if err := writeJSONValue(stdout, result); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return result.ExitCode
	}

	_, _ = io.WriteString(stdout, result.Stdout)
	if result.Stderr != "" {
		_, _ = io.WriteString(stderr, result.Stderr)
	}
	if result.Truncated {
		_, _ = fmt.Fprintf(stderr, "%s: output truncated (exceeded the server's per-response cap)\n", prog)
	}
	return result.ExitCode
}

func appsExecUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps exec <name> -- <command> [args...] [flags]

Runs command inside app <name>'s currently running container and waits
for it to finish (not an interactive shell: no PTY, no resize, nothing
kept alive between calls). Prints the command's real stdout/stderr and
exits this process with the command's own real exit code, not a generic
CLI success/failure code, so a caller can script against it directly
("%[1]s apps exec web -- test -f /ready.lock && echo ready").

The "--" before the command is required whenever the command itself
takes flags (anything starting with "-"): without it, this CLI's own
flag parser will try to consume that flag as its own and reject it as
unknown. Always safe to include, so every example here does.

Flags:
  --timeout int            command timeout in seconds, default %[5]d (the server enforces its own hard ceiling; this can only ask for less)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the full exec result (stdout, stderr, exit_code, truncated) as JSON to stdout, nothing else
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, defaultExecTimeoutSeconds)
}
