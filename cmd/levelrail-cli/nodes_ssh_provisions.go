package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"
)

// runNodesSSHProvisions dispatches "nodes ssh-provisions <verb> [flags]"
// to one of list/show: internal/api/node_ssh_provision.go's provision-
// tracking routes, the same two-level shape runNodesProvisions
// establishes for the cloud path.
func runNodesSSHProvisions(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, nodesSSHProvisionsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, nodesSSHProvisionsUsage(prog))
		return exitOK
	case "list":
		return runNodesSSHProvisionsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "show":
		return runNodesSSHProvisionsShow(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown nodes ssh-provisions subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, nodesSSHProvisionsUsage(prog))
		return exitUsage
	}
}

func nodesSSHProvisionsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes ssh-provisions list [flags]        list every SSH node provision, last known status
  %[1]s nodes ssh-provisions show <id> [flags]   show one provision, including its install log, refreshed live against the node registry

Run "%[1]s nodes ssh-provisions <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runNodesSSHProvisionsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes ssh-provisions list", "print provisions as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesSSHProvisionsListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	provisions, err := client.ListSSHNodeProvisions(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list ssh node provisions: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, provisions, func() { printSSHNodeProvisionsTable(stdout, provisions) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func nodesSSHProvisionsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes ssh-provisions list [flags]

Lists every SSH node provision, newest first, at its last known status
(not live-refreshed; use "nodes ssh-provisions show <id>" for that).

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print provisions as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func printSSHNodeProvisionsTable(out io.Writer, provisions []sshNodeProvisionResource) {
	if len(provisions) == 0 {
		_, _ = fmt.Fprintln(out, "no provisions")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tROLE\tSTATUS\tNODE ID\tCREATED")
	for _, p := range provisions {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", p.ID, p.Name, p.Role, p.Status, p.NodeID, p.CreatedAt.Format(time.RFC3339))
	}
	_ = tw.Flush()
}

func runNodesSSHProvisionsShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes ssh-provisions show", "print the provision as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesSSHProvisionsShowUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "nodes ssh-provisions show", "provision id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	p, err := client.GetSSHNodeProvision(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get ssh node provision %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, p, func() { printSSHNodeProvisionHuman(stdout, p) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func nodesSSHProvisionsShowUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes ssh-provisions show <id> [flags]

Shows one provision's current state, including its accumulated install
log, recomputed live against the node registry before returning (has the
expected node enrolled yet?).

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the provision as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func printSSHNodeProvisionHuman(out io.Writer, p sshNodeProvisionResource) {
	_, _ = fmt.Fprintf(out, "id:              %s\n", p.ID)
	_, _ = fmt.Fprintf(out, "name:            %s\n", p.Name)
	_, _ = fmt.Fprintf(out, "role:            %s\n", p.Role)
	_, _ = fmt.Fprintf(out, "status:          %s\n", p.Status)
	if p.DetectedOS != "" {
		_, _ = fmt.Fprintf(out, "detected os:     %s\n", p.DetectedOS)
	}
	if p.DetectedArch != "" {
		_, _ = fmt.Fprintf(out, "detected arch:   %s\n", p.DetectedArch)
	}
	if p.NodeID != "" {
		_, _ = fmt.Fprintf(out, "node id:         %s\n", p.NodeID)
	}
	if p.FailureReason != "" {
		_, _ = fmt.Fprintf(out, "failure reason:  %s\n", p.FailureReason)
	}
	_, _ = fmt.Fprintf(out, "created at:      %s\n", p.CreatedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(out, "updated at:      %s\n", p.UpdatedAt.Format(time.RFC3339))
	if p.Log != "" {
		_, _ = fmt.Fprintf(out, "log:\n%s\n", p.Log)
	}
}
