package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runDeployApprovals dispatches "deploy-approvals <verb> [args] [flags]"
// to one of list/get/approve/reject: the CLI surface for the two-person
// approval gate a deploy/promote into a protected environment now goes
// through (internal/api/deploy_approvals.go), replacing the old
// same-actor confirm flag.
func runDeployApprovals(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, deployApprovalsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, deployApprovalsUsage(prog))
		return exitOK
	case "list":
		return runDeployApprovalsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runDeployApprovalsGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "approve":
		return runDeployApprovalsApprove(prog, args[1:], stdout, stderr, lookupEnv)
	case "reject":
		return runDeployApprovalsReject(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown deploy-approvals subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, deployApprovalsUsage(prog))
		return exitUsage
	}
}

func deployApprovalsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s deploy-approvals list [--status STATUS] [--service NAME] [flags]   list pending (default) or decided approvals
  %[1]s deploy-approvals get <id> [flags]                                    show one approval
  %[1]s deploy-approvals approve <id> [flags]                                approve: the gated deploy/promote runs now
  %[1]s deploy-approvals reject <id> [--reason TEXT] [flags]                 reject: the app's desired state is left untouched

A deploy or promote into an environment tagged "protected" does not
apply immediately even with --confirm: it becomes a pending approval a
different, sufficiently privileged user must approve first. The same
user or token that requested it cannot approve or reject its own
request.

Run "%[1]s deploy-approvals <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runDeployApprovalsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "deploy-approvals list", "print approvals as a JSON array to stdout and nothing else", stderr)
	var status, service string
	fs.StringVar(&status, "status", "", "filter by status: pending (default), all, approved, rejected, or expired")
	fs.StringVar(&service, "service", "", "filter to one app name")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, deployApprovalsListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	approvals, err := client.ListDeployApprovals(context.Background(), status, service)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list deploy approvals: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, approvals, func() { printDeployApprovalsTable(stdout, approvals) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printDeployApprovalsTable(out io.Writer, approvals []deployApprovalResource) {
	if len(approvals) == 0 {
		_, _ = fmt.Fprintln(out, "no matching deploy approvals")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tACTION\tSERVICE\tIMAGE\tSTATUS\tREQUESTED BY\tCREATED")
	for _, a := range approvals {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", a.ID, a.Action, a.ServiceName, a.Image, a.Status, a.RequestedByName, a.CreatedAt)
	}
	_ = tw.Flush()
}

func deployApprovalsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s deploy-approvals list [--status STATUS] [--service NAME] [flags]

Flags:
  --status string          filter by status: pending (default), all, approved, rejected, or expired
  --service string        filter to one app name
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print approvals as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runDeployApprovalsGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "deploy-approvals get", "print the approval as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, deployApprovalsGetUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "deploy-approvals get", "approval id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	a, err := client.GetDeployApproval(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get deploy approval %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, a, func() { printDeployApprovalHuman(stdout, a) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func deployApprovalsGetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s deploy-approvals get <id> [flags]

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the approval as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runDeployApprovalsApprove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "deploy-approvals approve", "print the decision result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, deployApprovalsApproveUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "deploy-approvals approve", "approval id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.ApproveDeployApproval(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("approve deploy approval %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, result, func() {
		_, _ = fmt.Fprintf(stderr, "approval %q approved; app %q now targets image %q; reconcile is asynchronous, check \"%s apps status %s\"\n", id, result.App.Name, result.App.Image, prog, result.App.Name)
		printAppHuman(stdout, result.App)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func deployApprovalsApproveUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s deploy-approvals approve <id> [flags]

Approves a pending deploy approval: the gated deploy/promote runs now,
through the normal reconcile path. Fails with 403 if you are the same
user or token that requested it, or 409 if it is no longer pending.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the decision result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runDeployApprovalsReject(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "deploy-approvals reject", "print the rejected approval as JSON to stdout and nothing else", stderr)
	var reason string
	fs.StringVar(&reason, "reason", "", "optional note explaining the rejection")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, deployApprovalsRejectUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "deploy-approvals reject", "approval id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	a, err := client.RejectDeployApproval(context.Background(), id, reason)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("reject deploy approval %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, a, func() {
		_, _ = fmt.Fprintf(stdout, "approval %q rejected\n", id)
		printDeployApprovalHuman(stdout, a)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func deployApprovalsRejectUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s deploy-approvals reject <id> [--reason TEXT] [flags]

Rejects a pending deploy approval. The app's desired state is left
untouched; it never reaches reconcile.

Flags:
  --reason string          optional note explaining the rejection
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the rejected approval as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// printDeployApprovalHuman renders one deploy approval's key fields.
func printDeployApprovalHuman(out io.Writer, a deployApprovalResource) {
	_, _ = fmt.Fprintf(out, "id:            %s\n", a.ID)
	_, _ = fmt.Fprintf(out, "action:        %s\n", a.Action)
	_, _ = fmt.Fprintf(out, "service:       %s\n", a.ServiceName)
	if a.SourceServiceName != "" {
		_, _ = fmt.Fprintf(out, "source:        %s\n", a.SourceServiceName)
	}
	_, _ = fmt.Fprintf(out, "image:         %s\n", a.Image)
	_, _ = fmt.Fprintf(out, "status:        %s\n", a.Status)
	_, _ = fmt.Fprintf(out, "requested by:  %s\n", a.RequestedByName)
	_, _ = fmt.Fprintf(out, "created:       %s\n", a.CreatedAt)
	_, _ = fmt.Fprintf(out, "expires:       %s\n", a.ExpiresAt)
	if a.DecidedAt != "" {
		_, _ = fmt.Fprintf(out, "decided by:    %s\n", a.ApprovedByName)
		_, _ = fmt.Fprintf(out, "decided at:    %s\n", a.DecidedAt)
	}
	if a.Reason != "" {
		_, _ = fmt.Fprintf(out, "reason:        %s\n", a.Reason)
	}
}

// printDeployApprovalPendingHuman is apps_deploy.go/apps_promote.go's own
// output when a deploy/promote came back as a pending approval instead of
// applying: distinct from printAppHuman so a caller scripting on --json
// still gets a normal deployApprovalResource, and a human at a terminal
// sees plainly that nothing has actually deployed yet.
func printDeployApprovalPendingHuman(out io.Writer, prog string, a deployApprovalResource) {
	_, _ = fmt.Fprintf(out, "deploy of %q to image %q is pending approval (id %s)\n", a.ServiceName, a.Image, a.ID)
	_, _ = fmt.Fprintln(out, "the target environment is protected: a different, sufficiently privileged user must approve it before it deploys.")
	_, _ = fmt.Fprintf(out, "check status: %s deploy-approvals get %s\n", prog, a.ID)
}
