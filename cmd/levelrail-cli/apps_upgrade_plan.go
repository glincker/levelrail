package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsUpgradePlan prints what upgrading a template app to the current
// catalog entry would change. Read-only: nothing is applied.
func runAppsUpgradePlan(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps upgrade-plan", "print the plan as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps upgrade-plan <app-name> [flags]\n\nCompares an app deployed from a catalog template with the current template\nand prints the risk, image moves, env, volume and port changes, and any\nblockers. Nothing is changed.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps upgrade-plan", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	plan, err := client.GetTemplateUpgradePlan(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("upgrade plan for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, plan, func() { printTemplateUpgradePlan(stdout, plan) })
}

func printTemplateUpgradePlan(out io.Writer, p apiclient.TemplateUpgradePlan) {
	_, _ = fmt.Fprintf(out, "template: %s   risk: %s   automatic: %t\n", p.TemplateID, p.Risk, p.Automatic)
	if !p.HasChanges {
		_, _ = fmt.Fprintln(out, "up to date")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SERVICE\tSTATUS\tIMAGE\tENV\tVOLUMES\tPORTS")
	for _, s := range p.Services {
		img := "-"
		if s.Image != nil {
			img = s.Image.Kind
			if s.Image.From != "" && s.Image.To != "" {
				img += " " + s.Image.From + " -> " + s.Image.To
			}
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\n", s.Name, s.Status, img, len(s.Env), len(s.Volumes), len(s.Ports))
	}
	_ = tw.Flush()
	for _, b := range p.Blockers {
		_, _ = fmt.Fprintf(out, "BLOCKER %s (%s): %s\n", b.Code, b.Service, b.Detail)
	}
}
