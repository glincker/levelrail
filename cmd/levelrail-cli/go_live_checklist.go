package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	checklistLive           = "live"
	defaultChecklistTimeout = 5 * time.Minute
)

// checklistPoll is a variable so tests do not wait.
var checklistPoll = 5 * time.Second

func goLiveChecklistUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s go-live <app> [domain] [--wait] [--timeout 5m] [--json] [flags]

Prints the go live checklist for an app and domain: the app is healthy, its
port answers, the server firewall allows 80 and 443, DNS points here, the
certificate is issued and https://<domain> serves the app. Each step shows
its status, why, and the one thing to do next. The domain defaults to the
app's only domain. Without --wait the exit code is 0 whatever the state.
With --wait the command polls and exits 7 until the domain is live or
--timeout passes. Applying fixes happens in the dashboard or with
"fixes apply", "domains cert" and "apps domains dns".

Flags:
`, prog)
}

func runGoLiveChecklist(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "go-live", "print the checklist as JSON to stdout and nothing else", stderr)
	var wait bool
	var timeout time.Duration
	fs.BoolVar(&wait, "wait", false, "poll until the domain is live; exit 7 when it is not within --timeout")
	fs.DurationVar(&timeout, "timeout", defaultChecklistTimeout, "how long --wait polls")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, goLiveChecklistUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) < 1 || len(rest) > 2 {
		_, _ = fmt.Fprintf(stderr, "%s: go-live requires an app and optionally a domain\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	domain := ""
	if len(rest) == 2 {
		domain = strings.ToLower(rest[1])
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	for {
		cl, err := client.GoLiveChecklist(ctx, rest[0], domain)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("go live checklist for %q: %w", rest[0], err))
		}
		done := cl.State == checklistLive || !wait || !time.Now().Before(deadline)
		if !done {
			time.Sleep(checklistPoll)
			continue
		}
		if code := writeScheduledTaskResult(stdout, stderr, of, cl, func() { printChecklist(stdout, cl) }); code != exitOK {
			return code
		}
		if wait && cl.State != checklistLive {
			return exitNotLive
		}
		return exitOK
	}
}

var statusMarks = map[string]string{"ok": "ok  ", "warn": "warn", "fail": "FAIL", "pending": "wait", "unknown": "?   "}

func printChecklist(out io.Writer, cl apiclient.GoLiveChecklist) {
	target := cl.App
	if cl.Domain != "" {
		target += " on " + cl.Domain
	}
	_, _ = fmt.Fprintf(out, "go live: %s (%s)\n", target, strings.ReplaceAll(cl.State, "_", " "))
	for i, s := range cl.Steps {
		_, _ = fmt.Fprintf(out, "%d. [%s] %s\n", i+1, statusMarks[s.Status], s.Title)
		_, _ = fmt.Fprintf(out, "     %s\n", s.Summary)
		for _, e := range s.Evidence {
			if e.Value != "" {
				_, _ = fmt.Fprintf(out, "     %s: %s\n", e.Label, e.Value)
			}
		}
		if s.Scope != "" {
			_, _ = fmt.Fprintln(out, "     (checked from this server: it cannot prove the internet can reach it)")
		}
		printStepNext(out, s)
	}
	if cl.Live {
		_, _ = fmt.Fprintf(out, "\nlive: %s\n", cl.URL)
	}
}

func printStepNext(out io.Writer, s apiclient.ChecklistStep) {
	if s.Status == "ok" {
		return
	}
	if s.NextCheckAt != nil {
		_, _ = fmt.Fprintf(out, "     next check: %s\n", s.NextCheckAt.Format(time.RFC3339))
	}
	if f := s.FirewallFix; f != nil && len(f.Commands) > 0 {
		_, _ = fmt.Fprintf(out, "     fix (%s): %s\n     undo: %s\n", f.Kind, strings.Join(f.Commands, " && "), strings.Join(f.Undo, " && "))
		if f.Refusal != "" {
			_, _ = fmt.Fprintf(out, "     not automatic: %s\n", f.Refusal)
		}
	}
	if s.Provider != nil && s.Status != "pending" {
		_, _ = fmt.Fprintf(out, "     %s firewall: %s\n", s.Provider.Name, s.Provider.DocsURL)
	}
	if p := s.Primary; p != nil && p.Kind != "recheck" {
		line := p.Label
		switch {
		case p.Href != "":
			line += ": " + p.Href
		case p.Value != "":
			line += ": " + p.Value
		case p.FixID != "":
			line += " (fixes apply " + p.FixID + ")"
		}
		_, _ = fmt.Fprintf(out, "     next: %s\n", line)
	}
}
