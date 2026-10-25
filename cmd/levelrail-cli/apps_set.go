package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// onOffFlag parses "on" or "off" and records that it was set.
type onOffFlag struct {
	set   bool
	value bool
}

func (f *onOffFlag) String() string {
	if !f.set {
		return ""
	}
	if f.value {
		return "on"
	}
	return "off"
}

func (f *onOffFlag) Set(s string) error {
	switch s {
	case "on":
		f.value = true
	case "off":
		f.value = false
	default:
		return fmt.Errorf("must be on or off")
	}
	f.set = true
	return nil
}

func (f *onOffFlag) ptr() *bool {
	if !f.set {
		return nil
	}
	v := f.value
	return &v
}

// runAppsSet implements "apps set <name> --pr-comment-commands on|off".
func runAppsSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps set", "print the resulting settings as JSON to stdout and nothing else", stderr)
	var commands, forks onOffFlag
	fs.Var(&commands, "pr-comment-commands", "on or off: let write-access users run /levelrail commands from pull request comments")
	fs.Var(&forks, "pr-comment-commands-forks", "on or off: also accept those commands on pull requests from forks")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsSetUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "apps set", "app name")
	if !ok {
		return exitUsage
	}
	if !commands.set && !forks.set {
		return reportError(stdout, stderr, jsonOut, newValidationError("apps set needs --pr-comment-commands or --pr-comment-commands-forks"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.SetPRCommentCommands(context.Background(), name, commands.ptr(), forks.ptr())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set pull request comment commands for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printPRCommentCommands(stdout, name, res) })
}

func printPRCommentCommands(out io.Writer, name string, res apiclient.PreviewSettingsResource) {
	state := map[bool]string{true: "on", false: "off"}
	_, _ = fmt.Fprintf(out, "app %q: pull request comment commands %s, fork pull requests %s\n", name, state[res.PRCommentCommands], state[res.PRCommentCommandsForks])
}

func appsSetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps set <app-name> [--pr-comment-commands on|off] [--pr-comment-commands-forks on|off] [flags]

Changes app settings. Pull request comment commands let a user with write
access to the repository run "/levelrail redeploy", "/levelrail rollback",
"/levelrail status" and "/levelrail preview redeploy" from a pull request
comment. They are off by default, and the control plane's
APP_PR_COMMENT_COMMANDS_DISABLED switch overrides this setting.

Flags:
  --pr-comment-commands string        on or off: accept /levelrail commands from pull request comments
  --pr-comment-commands-forks string  on or off: also accept them on pull requests from forks (default off)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the resulting settings as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
