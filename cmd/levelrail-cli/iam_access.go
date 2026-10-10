package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const iamAPIFlagsHelp = `  --token string       API token (default: %[1]s env var, then the credentials file)
  --api-url string    control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json              print the result as JSON to stdout, nothing else
  --output string     output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string      JMESPath expression to filter the result before printing
  -h, --help          show this help
`

// parsePrincipal splits "user:ID" or "token:ID".
func parsePrincipal(s string) (ptype, id string, err error) {
	ptype, id, ok := strings.Cut(s, ":")
	if !ok || id == "" || (ptype != "user" && ptype != "token") {
		return "", "", newValidationError("--principal %q must look like user:ID or token:ID", s)
	}
	return ptype, id, nil
}

func runIAMSimulate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "iam simulate", "print the decision as JSON to stdout and nothing else", stderr)
	var principal, action, resource string
	fs.StringVar(&principal, "principal", "", "who to test, user:ID or token:ID")
	fs.StringVar(&action, "action", "", "ability to test, e.g. write")
	fs.StringVar(&resource, "resource", "", "resource to test, e.g. app:web")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, iamSimulateUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	ptype, pid, err := parsePrincipal(principal)
	if err == nil && (action == "" || resource == "") {
		err = newValidationError("--action and --resource are required")
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	sim, err := client.SimulateIAM(context.Background(), ptype, pid, action, resource)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("simulate %s on %s: %w", action, resource, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, sim, func() { printSimulation(stdout, sim) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printSimulation(out io.Writer, s apiclient.IAMSimulation) {
	verdict := "DENY"
	if s.Allowed {
		verdict = "ALLOW"
	}
	_, _ = fmt.Fprintf(out, "%s: %s, %s on %s (%s)\n", verdict, s.PrincipalName, s.Action, s.Resource, s.DecidedBy)
	if d := s.DecidingStatement; d != nil {
		_, _ = fmt.Fprintf(out, "decided by policy %s, statement %d: %s %s on %s\n", d.PolicyName, d.StatementIndex+1, d.Effect, strings.Join(d.Action, ","), strings.Join(d.Resource, ","))
	}
	for _, m := range s.MatchedStatements {
		_, _ = fmt.Fprintf(out, "  matched %s statement %d: %s %s on %s\n", m.PolicyName, m.StatementIndex+1, m.Effect, strings.Join(m.Action, ","), strings.Join(m.Resource, ","))
	}
}

func iamSimulateUsage(prog string) string {
	return fmt.Sprintf("Usage:\n  %[1]s iam simulate --principal user:ID|token:ID --action ABILITY --resource RESOURCE [flags]\n\nAsk whether a principal can do an ability on a resource, using the platform's real evaluator.\nNothing is changed.\n\nFlags:\n  --principal string   who to test, user:ID or token:ID\n  --action string      ability, e.g. write\n  --resource string    resource, e.g. app:web or database:main\n", prog) +
		fmt.Sprintf(iamAPIFlagsHelp, envAPIToken, envAPIURL, defaultAPIURL)
}

func runIAMEffective(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "iam effective", "print the permissions as JSON to stdout and nothing else", stderr)
	var principal string
	fs.StringVar(&principal, "principal", "", "who to inspect, user:ID or token:ID")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, iamEffectiveUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	ptype, pid, err := parsePrincipal(principal)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	eff, err := client.IAMEffectivePermissions(context.Background(), ptype, pid)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("effective permissions for %s: %w", principal, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, eff, func() { printEffective(stdout, eff) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printEffective(out io.Writer, e apiclient.IAMEffective) {
	_, _ = fmt.Fprintf(out, "%s (%s)\n", e.Principal.Name, e.Principal.PrincipalType)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ABILITY\tRISK\tALLOWED\tVIA POLICY\tDENIED BY POLICY")
	for _, a := range e.Abilities {
		scope := fmt.Sprintf("%d of %d", a.Allowed, a.Total)
		if a.All {
			scope = "all"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\n", a.Ability, a.Risk, scope, a.GrantedByPolicy, a.DeniedByPolicy)
	}
	_ = tw.Flush()
}

func iamEffectiveUsage(prog string) string {
	return fmt.Sprintf("Usage:\n  %[1]s iam effective --principal user:ID|token:ID [flags]\n\nShow what a principal can do on every app and database, grouped by ability.\n\nFlags:\n  --principal string   who to inspect, user:ID or token:ID\n", prog) +
		fmt.Sprintf(iamAPIFlagsHelp, envAPIToken, envAPIURL, defaultAPIURL)
}

func runIAMAnalyze(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "iam analyze", "print the findings as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, iamAnalyzeUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.AnalyzeIAM(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("analyze iam: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printAnalysis(stdout, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printAnalysis(out io.Writer, a apiclient.IAMAnalysis) {
	_, _ = fmt.Fprintf(out, "IAM score %d/100, %d finding(s)\n", a.Score, len(a.Findings))
	for _, f := range a.Findings {
		_, _ = fmt.Fprintf(out, "[%s] %s\n    fix: %s\n", f.Severity, f.Message, f.Fix)
	}
}

func iamAnalyzeUsage(prog string) string {
	return fmt.Sprintf("Usage:\n  %[1]s iam analyze [flags]\n\nRun static checks over every policy, attachment and token, with a one line fix for each finding.\n\nFlags:\n", prog) +
		fmt.Sprintf(iamAPIFlagsHelp, envAPIToken, envAPIURL, defaultAPIURL)
}
