package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"
)

// runNodesProvisions dispatches "nodes provisions <verb> [flags]" to one
// of list/show: internal/api/node_provision.go's provision-tracking
// routes, the same two-level shape runNodesProviders establishes.
func runNodesProvisions(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, nodesProvisionsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, nodesProvisionsUsage(prog))
		return exitOK
	case "list":
		return runNodesProvisionsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "show":
		return runNodesProvisionsShow(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown nodes provisions subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, nodesProvisionsUsage(prog))
		return exitUsage
	}
}

func nodesProvisionsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes provisions list [flags]        list every cloud node provision, last known status
  %[1]s nodes provisions show <id> [flags]   show one provision, refreshed live against the provider and the node registry

Run "%[1]s nodes provisions <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runNodesProvisionsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes provisions list", "print provisions as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesProvisionsListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	provisions, err := client.ListNodeProvisions(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list node provisions: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, provisions, func() { printNodeProvisionsTable(stdout, provisions) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func nodesProvisionsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes provisions list [flags]

Lists every cloud node provision, newest first, at its last known status
(not live-refreshed; use "nodes provisions show <id>" for that).

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

func printNodeProvisionsTable(out io.Writer, provisions []nodeProvisionResource) {
	if len(provisions) == 0 {
		_, _ = fmt.Fprintln(out, "no provisions")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tPROVIDER\tNAME\tROLE\tSTATUS\tNODE ID\tCREATED")
	for _, p := range provisions {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", p.ID, p.Provider, p.Name, p.Role, p.Status, p.NodeID, p.CreatedAt.Format(time.RFC3339))
	}
	_ = tw.Flush()
}

func runNodesProvisionsShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes provisions show", "print the provision as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesProvisionsShowUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "nodes provisions show", "provision id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	p, err := client.GetNodeProvision(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get node provision %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, p, func() { printNodeProvisionHuman(stdout, p) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func nodesProvisionsShowUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes provisions show <id> [flags]

Shows one provision's current state, recomputed live against the
provider's API and the node registry before returning.

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

func printNodeProvisionHuman(out io.Writer, p nodeProvisionResource) {
	_, _ = fmt.Fprintf(out, "id:              %s\n", p.ID)
	_, _ = fmt.Fprintf(out, "provider:        %s\n", p.Provider)
	_, _ = fmt.Fprintf(out, "region:          %s\n", p.Region)
	_, _ = fmt.Fprintf(out, "size:            %s\n", p.Size)
	_, _ = fmt.Fprintf(out, "name:            %s\n", p.Name)
	_, _ = fmt.Fprintf(out, "role:            %s\n", p.Role)
	_, _ = fmt.Fprintf(out, "status:          %s\n", p.Status)
	if p.IPAddress != "" {
		_, _ = fmt.Fprintf(out, "ip address:      %s\n", p.IPAddress)
	}
	if p.NodeID != "" {
		_, _ = fmt.Fprintf(out, "node id:         %s\n", p.NodeID)
	}
	if p.FailureReason != "" {
		_, _ = fmt.Fprintf(out, "failure reason:  %s\n", p.FailureReason)
	}
	_, _ = fmt.Fprintf(out, "created at:      %s\n", p.CreatedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(out, "updated at:      %s\n", p.UpdatedAt.Format(time.RFC3339))
}
