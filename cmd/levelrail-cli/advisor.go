package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/resourceadvisor"
	"github.com/GLINCKER/levelrail/internal/rightsizing"
)

// runAdvisor dispatches "advisor <verb>": the beta resource advisor.
func runAdvisor(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, advisorUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, advisorUsage(prog))
		return exitOK
	case "findings":
		return runAdvisorFindings(prog, args[1:], stdout, stderr, lookupEnv)
	case "summary":
		return runAdvisorSummary(prog, args[1:], stdout, stderr, lookupEnv)
	case "apply":
		return runAdvisorApply(prog, args[1:], stdout, stderr, lookupEnv)
	case "dismiss", "snooze", "restore":
		return runAdvisorState(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	case "enable", "disable":
		return runAdvisorSwitch(prog, args[0] == "enable", args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown advisor subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, advisorUsage(prog))
		return exitUsage
	}
}

func advisorUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s advisor findings [--resource app:NAME] [--severity S] [--all]   list resource advice (beta)
  %[1]s advisor summary                                              counts and the biggest savings
  %[1]s advisor apply <finding-id> [--yes]                           preview, then apply with --yes
  %[1]s advisor dismiss <finding-id>                                 hide a finding until restored
  %[1]s advisor snooze <finding-id> [--for 7d]                       hide a finding for a while
  %[1]s advisor restore <finding-id>                                 show a dismissed or snoozed finding again
  %[1]s advisor enable|disable                                       switch the beta on or off for this instance

The resource advisor is in beta and off by default. It only suggests:
nothing changes until you run apply with --yes.
`, prog)
}

func formatAdvisorValue(a *resourceadvisor.Action, v int64) string {
	if a == nil {
		return ""
	}
	switch a.Type {
	case resourceadvisor.ActionSetMemoryLimit:
		if v == 0 {
			return "no limit"
		}
		return rightsizing.FormatMemory(v)
	case resourceadvisor.ActionSetCPULimit:
		if v == 0 {
			return "no limit"
		}
		return rightsizing.FormatCPU(v)
	case resourceadvisor.ActionEnableSleep:
		if v == 0 {
			return "never sleeps"
		}
		return fmt.Sprintf("sleep after %d idle minutes", v)
	}
	return ""
}

func runAdvisorFindings(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "advisor findings", "print the findings as JSON to stdout and nothing else", stderr)
	resource := fs.String("resource", "", "only findings for this resource, such as app:web or database:main")
	severity := fs.String("severity", "", "only findings at or above this severity: info, suggest or warn")
	all := fs.Bool("all", false, "include dismissed and snoozed findings")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.ListAdvisorFindings(context.Background(), apiclient.AdvisorFindingQuery{Resource: *resource, Severity: *severity, IncludeHidden: *all})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list advisor findings: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printAdvisorFindings(stdout, res.Findings) })
}

func printAdvisorFindings(out io.Writer, fs []apiclient.AdvisorFinding) {
	if len(fs) == 0 {
		_, _ = fmt.Fprintln(out, "no advice right now")
		return
	}
	for _, f := range fs {
		state := ""
		if f.State != "" {
			state = " (" + f.State + ")"
		}
		_, _ = fmt.Fprintf(out, "[%s] %s%s\n  id: %s\n  %s\n", f.Severity, f.Title, state, f.ID, f.Why)
		if f.Action != nil {
			if f.Appliable {
				_, _ = fmt.Fprintf(out, "  action: %s -> %s\n", formatAdvisorValue(f.Action, f.Action.Current), formatAdvisorValue(f.Action, f.Action.Value))
			}
			keys := make([]string, 0, len(f.Action.Settings))
			for k := range f.Action.Settings {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				_, _ = fmt.Fprintf(out, "  %s = %s\n", k, f.Action.Settings[k])
			}
			if f.Action.Instructions != "" {
				_, _ = fmt.Fprintf(out, "  how: %s\n", f.Action.Instructions)
			}
		}
		_, _ = fmt.Fprintln(out)
	}
}

func runAdvisorSummary(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "advisor summary", "print the summary as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetAdvisorSummary(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get advisor summary: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "findings: %d (warn %d, suggest %d, info %d), %d can be applied\n", res.Total, res.BySeverity["warn"], res.BySeverity["suggest"], res.BySeverity["info"], res.Actionable)
		if res.PendingData > 0 {
			_, _ = fmt.Fprintf(stdout, "not enough data yet: %d resources\n", res.PendingData)
		}
		if res.SavingMemoryBytes > 0 {
			_, _ = fmt.Fprintf(stdout, "estimated memory to reclaim: %s\n", rightsizing.FormatMemory(res.SavingMemoryBytes))
		}
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		for _, f := range res.TopSavings {
			_, _ = fmt.Fprintf(tw, "  %s\t%s\n", rightsizing.FormatMemory(f.Impact.SavingMemoryBytes+f.Impact.SavingDiskBytes), f.Title)
		}
		_ = tw.Flush()
	})
}

func runAdvisorApply(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "advisor apply", "print the preview or result as JSON to stdout and nothing else", stderr)
	yes := fs.Bool("yes", false, "apply the change; without it only the preview is shown")
	client, id, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "advisor apply", "finding id"}, lookupEnv)
	if !ok {
		return exitCode
	}
	res, err := client.ApplyAdvisorFinding(context.Background(), id, *yes)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("apply advisor finding %q: %w", id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		var before, after int64
		switch res.Action.Type {
		case resourceadvisor.ActionSetMemoryLimit:
			before, after = res.Before.MemoryBytes, res.After.MemoryBytes
		case resourceadvisor.ActionSetCPULimit:
			before, after = res.Before.NanoCPUs, res.After.NanoCPUs
		case resourceadvisor.ActionEnableSleep:
			before, after = int64(res.Before.SleepIdleMinutes), int64(res.After.SleepIdleMinutes)
		}
		_, _ = fmt.Fprintf(stdout, "%s: %s -> %s\n", res.Resource, formatAdvisorValue(res.Action, before), formatAdvisorValue(res.Action, after))
		switch {
		case res.Applied && res.AppliedLive:
			_, _ = fmt.Fprintln(stdout, "applied, and updated on the running container")
		case res.Applied:
			_, _ = fmt.Fprintln(stdout, "applied; takes effect on the next start or deploy")
		default:
			_, _ = fmt.Fprintf(stdout, "preview only, nothing changed. Run %s advisor apply %s --yes to apply.\n", prog, id)
		}
	})
}

func runAdvisorState(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "advisor "+verb, "print the finding as JSON to stdout and nothing else", stderr)
	duration := ""
	var forFlag *string
	if verb == "snooze" {
		forFlag = fs.String("for", "7d", "how long to hide the finding, such as 24h or 7d (at most 90d)")
	}
	client, id, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "advisor " + verb, "finding id"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if forFlag != nil {
		duration = strings.TrimSpace(*forFlag)
	}
	res, err := client.SetAdvisorFindingState(context.Background(), id, verb, duration)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s advisor finding %q: %w", verb, id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		switch verb {
		case "snooze":
			_, _ = fmt.Fprintf(stdout, "snoozed %s until %s\n", id, res.SnoozedUntil)
		case "dismiss":
			_, _ = fmt.Fprintf(stdout, "dismissed %s\n", id)
		default:
			_, _ = fmt.Fprintf(stdout, "restored %s\n", id)
		}
	})
}

func runAdvisorSwitch(prog string, enable bool, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	verb := map[bool]string{true: "enable", false: "disable"}[enable]
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "advisor "+verb, "print the feature state as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.SetBetaFeature(context.Background(), string(experimental.ResourceAdvisor), enable)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s resource advisor: %w", verb, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		switch {
		case res.Locked:
			_, _ = fmt.Fprintln(stdout, "resource advisor is on through APP_EXPERIMENTAL; remove it there to turn it off")
		case res.Enabled:
			_, _ = fmt.Fprintln(stdout, "resource advisor (beta) is on")
		default:
			_, _ = fmt.Fprintln(stdout, "resource advisor is off")
		}
	})
}
