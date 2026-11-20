package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const pagesNotifyUsage = "Usage:\n  %[1]s pages notify show <site> [flags]\n  %[1]s pages notify set <site> [--on success,failure] [--channel ID] [flags]\n  %[1]s pages notify off <site> [flags]\n  %[1]s pages notify reset <site> [flags]\n"

// runPagesNotify manages deploy notifications of a Pages site through the shared notification channels.
func runPagesNotify(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, pagesNotifyUsage, prog)
		return exitUsage
	}
	verb, args := args[0], args[1:]
	if verb != "show" && verb != "set" && verb != "off" && verb != "reset" {
		_, _ = fmt.Fprintf(stderr, pagesNotifyUsage, prog)
		return exitUsage
	}
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "pages notify "+verb, "print the result as JSON to stdout and nothing else", stderr)
	on := fs.String("on", "", "events to notify on, comma separated: success, failure (set only, default keeps the stored filter)")
	channel := fs.String("channel", "", "notification channel ID to attach (set only, see the notification channels list)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	site, ok := requireOneArg(fs, stderr, prog, "pages notify "+verb, "site name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var res apiclient.PagesNotify
	var err error
	switch verb {
	case "reset":
		if err = client.ResetPagesNotify(ctx, site); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("reset notifications of %q: %w", site, err))
		}
		_, _ = fmt.Fprintf(stdout, "notifications of %s reset to every outcome on attached channels\n", site)
		return exitOK
	case "off":
		off := false
		res, err = client.PutPagesNotify(ctx, site, apiclient.PagesNotifyUpdate{Enabled: &off})
	case "set":
		upd := apiclient.PagesNotifyUpdate{ChannelID: *channel}
		enable := true
		upd.Enabled = &enable
		if *on != "" {
			upd.Events = strings.Split(*on, ",")
		}
		res, err = client.PutPagesNotify(ctx, site, upd)
	default:
		res, err = client.GetPagesNotify(ctx, site)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s notifications of %q: %w", verb, site, err))
	}
	return renderOrFail(stdout, stderr, of, res, func() {
		state := "on"
		if !res.Enabled {
			state = "off"
		}
		_, _ = fmt.Fprintf(stdout, "%s: notifications %s, events %s, %d channel(s)\n", site, state, strings.Join(res.Events, ","), len(res.Channels))
		for _, c := range res.Channels {
			_, _ = fmt.Fprintf(stdout, "  %s\n", c.ChannelID)
		}
	})
}
