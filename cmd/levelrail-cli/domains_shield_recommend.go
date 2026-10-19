package main

import (
	"fmt"
	"io"
)

func runShieldRecommend(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "domains shield recommend", policyJSONUsage, stderr)
	var dryRun, blockBots bool
	fs.BoolVar(&dryRun, "dry-run", false, "show what would change without writing anything")
	fs.BoolVar(&blockBots, "block-bots", false, "also refuse the shipped bad bot user agents")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains shield recommend <app> [--dry-run] [--block-bots] [flags]\n\nTurns on sensible per-client limits for each of the app's domains that has no shield.\nDomains that already have a shield are left alone. Refused while a proxy hides client IPs.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, app, jsonOut, of, code, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, stderr, singleArgCmd{prog, "domains shield recommend", "app name"}, lookupEnv)
	if !ok {
		return code
	}
	res, err := client.ApplyRecommendedShield(bg(), app, dryRun, blockBots)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("recommended shield for app %q: %w", app, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		if res.Blocked != "" {
			_, _ = fmt.Fprintln(stdout, "refused: a proxy in front hides client addresses, so a per-client limit would hit everyone. Set APP_INGRESS_TRUSTED_PROXIES first.")
			return
		}
		verb := "applied to"
		if res.DryRun {
			verb = "would apply to"
		}
		_, _ = fmt.Fprintf(stdout, "%d requests per minute, burst %d, %d concurrent per client; %s %d domain(s)\n", res.RatePerMinute, res.Burst, res.MaxConcurrent, verb, len(res.Applied))
		for _, d := range res.Applied {
			_, _ = fmt.Fprintf(stdout, "  %s\n", d)
		}
		for _, s := range res.Skipped {
			_, _ = fmt.Fprintf(stdout, "  %s: skipped (%s)\n", s.Domain, s.Reason)
		}
	})
}
