package main

import (
	"context"
	"fmt"
	"io"
)

func settingsDeployURLCopiesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings deploy-url-copies get [flags]
  %[1]s settings deploy-url-copies set <off|private|public|default> [flags]

Whether recent deploys keep a retained copy, and whether it is reachable.
  off      no copies are kept
  private  copies are kept suspended with no public host or certificate (default)
  public   copies get their own hostname and certificate, the pre-0.9 behavior
  default  clear the stored value; APP_DEPLOY_URL_COPIES, then private, applies

A stored value wins over the environment. It applies on the next sync pass,
without a restart.
`, prog)
}

func runSettingsDeployURLCopies(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsDeployURLCopiesUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsDeployURLCopiesUsage(prog))
		return exitOK
	case "get":
		return runListCommand(prog, args[1:], stdout, stderr, lookupEnv, listCommandParams[deployURLCopiesResource]{
			cmdLabel:  "settings deploy-url-copies get",
			jsonUsage: "print the setting as JSON to stdout and nothing else",
			usageText: fmt.Sprintf("Usage:\n  %s settings deploy-url-copies get [flags]\n\nShows the copies mode and where it comes from.\n\nFlags:\n", prog),
			fetch: func(c *Client, ctx context.Context) (deployURLCopiesResource, error) {
				return c.GetDeployURLCopies(ctx)
			},
			errVerb: "get deploy url copies",
			print:   printDeployURLCopiesHuman,
		})
	case "set":
		return runSettingsDeployURLCopiesSet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings deploy-url-copies subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsDeployURLCopiesUsage(prog))
		return exitUsage
	}
}

func runSettingsDeployURLCopiesSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings deploy-url-copies set", "print the setting as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, settingsDeployURLCopiesUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 1 {
		return reportError(stdout, stderr, jsonOut, newValidationError("exactly one mode is required: off, private, public or default"))
	}
	mode := fs.Arg(0)
	switch mode {
	case "off", "private", "public":
	case "default":
		mode = ""
	default:
		return reportError(stdout, stderr, jsonOut, newValidationError("mode must be off, private, public or default"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.SetDeployURLCopies(context.Background(), mode)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set deploy url copies: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printDeployURLCopiesHuman(stdout, res) })
}

func printDeployURLCopiesHuman(out io.Writer, r deployURLCopiesResource) {
	_, _ = fmt.Fprintf(out, "mode:    %s (%s)\n", r.Mode, r.Source)
	_, _ = fmt.Fprintf(out, "default: %s\n", r.Default)
}
