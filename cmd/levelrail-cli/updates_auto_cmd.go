package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runUpdatesAuto implements "updates status|enable|disable|channel|check|history|dismiss-notice".
// handled is false for any other argument, which keeps going to "upgrade".
func runUpdatesAuto(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "check":
		return runUpgrade(prog, rest, stdout, stderr, lookupEnv), true
	case "history":
		return runUpgrade(prog, append([]string{"--history"}, rest...), stdout, stderr, lookupEnv), true
	case "status", "enable", "disable", "channel", "dismiss-notice":
	default:
		return 0, false
	}
	channelName := ""
	if sub == "channel" {
		if len(rest) == 0 || (len(rest[0]) > 0 && rest[0][0] == '-') {
			_, _ = fmt.Fprintf(stderr, "usage: %s updates channel <stable|beta|edge> [flags]\n", prog)
			return exitUsage, true
		}
		channelName, rest = rest[0], rest[1:]
	}
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "updates "+sub, "print the result as JSON to stdout and nothing else", stderr)
	mode := fs.String("mode", "any", "with enable: patch (same minor version only) or any (any eligible release on the channel)")
	cron := fs.String("window-cron", "0 3 * * *", "with enable, when no window is set: maintenance window start as a cron `expression`")
	days := fs.Int("days", 0, "with disable: pause automatic updates for this many `days` instead of turning them off")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s updates status\n  %[1]s updates enable [--mode any|patch]\n  %[1]s updates disable [--days N]\n  %[1]s updates channel <name>\n  %[1]s updates check\n  %[1]s updates history\n  %[1]s updates dismiss-notice\n\nAutomatic updates are on by default. Every update is checksum and signature verified, backed up\nand health checked before it stays; a failed check keeps the running version and rolls back.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, rest, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode, true
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	if sub == "channel" {
		cur, err := client.GetUpdateSettings(ctx)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("update settings: %w", err)), true
		}
		cur.Channel = channelName
		out, err := client.SetUpdateSettings(ctx, cur)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set channel: %w", err)), true
		}
		return renderUpgradeResult(stdout, stderr, of, out, func() { printUpdateSettingsHuman(stdout, out) }), true
	}
	if sub == "dismiss-notice" {
		out, err := client.DismissUpgradeNotice(ctx)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("dismiss notice: %w", err)), true
		}
		return renderUpgradeResult(stdout, stderr, of, out, func() { printUpgradePolicy(stdout, out) }), true
	}
	cur, err := client.GetUpgradePolicy(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("upgrade policy: %w", err)), true
	}
	if sub == "status" {
		settings, serr := client.GetUpdateSettings(ctx)
		return renderUpgradeResult(stdout, stderr, of, cur, func() {
			printUpgradePolicy(stdout, cur)
			if serr == nil {
				_, _ = fmt.Fprintf(stdout, "channel: %s\n", settings.Channel)
			}
			printUpgradeNotice(stdout, prog, cur)
		}), true
	}
	req := apiclient.UpgradePolicyRequest{
		Mode: cur.Mode, WindowCron: cur.WindowCron, WindowDurationMinutes: cur.WindowDurationMinutes, Timezone: cur.Timezone,
		MinReleaseAgeDays: cur.MinReleaseAgeDays, NotifyChannelIDs: cur.NotifyChannelIDs, Paused: cur.Paused,
	}
	zero := 0
	switch sub {
	case "enable":
		req.Mode, req.Paused, req.PauseDays = *mode, false, &zero
		if req.WindowCron == "" {
			req.WindowCron = *cron
		}
	case "disable":
		if *days < 0 {
			return reportError(stdout, stderr, jsonOut, errors.New("--days must not be negative")), true
		}
		if *days > 0 {
			req.PauseDays = days
		} else {
			req.Mode = "off"
		}
	}
	out, err := client.SetUpgradePolicy(ctx, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s automatic updates: %w", sub, err)), true
	}
	return renderUpgradeResult(stdout, stderr, of, out, func() { printUpgradePolicy(stdout, out) }), true
}

// printUpgradeNotice prints the one-time default-on notice until it is dismissed.
func printUpgradeNotice(out io.Writer, prog string, p apiclient.UpgradePolicy) {
	if p.Notice == nil {
		return
	}
	_, _ = fmt.Fprintf(out, "\nnote: %s\n  turn off: %s updates disable   hide this note: %s updates dismiss-notice\n", p.Notice.Text, prog, prog)
}
