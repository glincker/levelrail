package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/coolifyexit"
	"github.com/GLINCKER/levelrail/internal/platformimport"
)

type coolifyExitFlags struct {
	url, token                                        string
	verify                                            bool
	insecureTLS, allowPrivate, allowLoopback, jsonOut bool
	only                                              string
}

// runCoolifyExit prints the exit report. Dry run reads Coolify from this machine and needs no
// target; verify asks the control plane, which knows which apps already exist there.
func runCoolifyExit(ctx context.Context, prog string, f coolifyExitFlags, client func() *Client, stdout, stderr io.Writer) int {
	var only []string
	for _, o := range strings.Split(f.only, ",") {
		if o = strings.TrimSpace(o); o != "" {
			only = append(only, o)
		}
	}
	var rep coolifyexit.Report
	if f.verify {
		var err error
		rep, err = client().CoolifyExitReport(ctx, apiclient.PlatformImportRequest{Platform: string(platformimport.Coolify), URL: f.url, Token: f.token,
			InsecureTLS: f.insecureTLS, AllowPrivate: f.allowPrivate, Only: only})
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("verification report: %w", err))
		}
	} else {
		src, err := platformimport.NewCoolify(f.url, f.token, platformimport.ClientOptions{Policy: platformimport.NetworkPolicy{AllowPrivate: f.allowPrivate, AllowLoopback: f.allowLoopback}, Insecure: f.insecureTLS})
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, newValidationError("%s", err.Error()))
		}
		disc, err := src.Discover(ctx)
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("read coolify: %w", err))
		}
		rep = coolifyexit.Build(disc, platformimport.BuildPlan(disc, platformimport.PlanOptions{Only: only}))
	}
	return printCoolifyExit(stdout, stderr, prog, f.jsonOut, rep)
}

func printCoolifyExit(stdout, stderr io.Writer, prog string, jsonOut bool, rep coolifyexit.Report) int {
	if jsonOut {
		if err := writeJSONValue(stdout, rep); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	renderCoolifyExit(stdout, prog, rep)
	return exitOK
}

func renderCoolifyExit(w io.Writer, prog string, rep coolifyexit.Report) {
	title := "Coolify exit plan (dry run, nothing was created)"
	if rep.Mode == coolifyexit.ModeVerify {
		title = "Coolify exit verification (what exists here now)"
	}
	_, _ = fmt.Fprintf(w, "%s\n%d imported, %d need manual action, %d skipped\n\n", title,
		rep.Counts[coolifyexit.Imported], rep.Counts[coolifyexit.Manual], rep.Counts[coolifyexit.Skipped])
	for _, out := range []coolifyexit.Outcome{coolifyexit.Skipped, coolifyexit.Manual, coolifyexit.Imported} {
		var rows []coolifyexit.Row
		for _, r := range rep.Rows {
			if r.Outcome == out {
				rows = append(rows, r)
			}
		}
		if len(rows) == 0 {
			continue
		}
		_, _ = fmt.Fprintf(w, "%s:\n", strings.ToUpper(string(out)))
		for _, r := range rows {
			target := ""
			if r.Target != "" && r.Target != r.Name {
				target = " -> " + r.Target
			}
			_, _ = fmt.Fprintf(w, "  [%s] %s%s\n", r.Kind, r.Name, target)
			for _, s := range r.Reasons {
				_, _ = fmt.Fprintf(w, "      why:  %s\n", s)
			}
			for _, s := range r.Next {
				_, _ = fmt.Fprintf(w, "      next: %s\n", prefixCommand(prog, s))
			}
		}
		_, _ = fmt.Fprintln(w)
	}
	for _, n := range rep.Notes {
		_, _ = fmt.Fprintf(w, "note: %s\n", n)
	}
	if len(rep.Waves) > 0 {
		_, _ = fmt.Fprintln(w, "DNS switch order:")
		for _, wv := range rep.Waves {
			_, _ = fmt.Fprintf(w, "  wave %d, %s\n      %s\n", wv.Number, strings.ToLower(wv.Title), wv.Why)
			for _, a := range wv.Apps {
				domains := append([]string(nil), a.Domains...)
				sort.Strings(domains)
				_, _ = fmt.Fprintf(w, "      - %s %s\n", a.Name, strings.Join(domains, ", "))
			}
		}
		_, _ = fmt.Fprintln(w)
	}
	_, _ = fmt.Fprintln(w, "Cutover checklist:")
	for i, s := range rep.Checklist {
		_, _ = fmt.Fprintf(w, "  %d. %s\n     %s\n", i+1, s.Title, s.Detail)
		for _, c := range s.Commands {
			_, _ = fmt.Fprintf(w, "     $ %s %s\n", prog, c)
		}
	}
	if rep.ReadyToRetire {
		_, _ = fmt.Fprintln(w, "\nEvery resource is imported and none needs action. Coolify can be retired after the last checklist step.")
	}
}

func envIsTrue(lookupEnv func(string) (string, bool), name string) bool {
	v, ok := lookupEnv(name)
	return ok && strings.EqualFold(strings.TrimSpace(v), "true")
}

// prefixCommand adds the program name to a hint that starts with a subcommand.
func prefixCommand(prog, hint string) string {
	for _, p := range []string{"migrate ", "import "} {
		if strings.HasPrefix(hint, p) {
			return prog + " " + hint
		}
	}
	return hint
}
