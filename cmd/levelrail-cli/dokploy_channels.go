package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/platformimport/dokployextras"
)

// Alert channel import; the planning and create calls live in dokployextras, shared with the dashboard import.
const (
	channelUnsupported = dokployextras.StateUnsupported
	journalKindChannel = dokployextras.KindChannel
)

type (
	channelImport      = dokployextras.ChannelImport
	importedChannel    = dokployextras.Channel
	unsupportedChannel = dokployextras.UnsupportedChannel
	channelPlanOptions = dokployextras.ChannelOptions
)

func channelPlanOptionsFromEnv(ctx context.Context, enable bool, lookupEnv func(string) (string, bool)) channelPlanOptions {
	return dokployextras.NewChannelOptions(ctx, enable, cliImportPolicy(lookupEnv), lookupEnv)
}

func planDokployChannels(disc *platformimport.Discovery, o channelPlanOptions) *channelImport {
	return dokployextras.PlanChannels(disc, o)
}

func channelFingerprint(ch notificationChannelResource) string {
	return dokployextras.ChannelFingerprint(ch)
}

func applyChannelImport(ctx context.Context, client *Client, imp *channelImport, jr *applyJournal) {
	dokployextras.ApplyChannels(ctx, client, imp, jr.extras())
}

func printChannelImport(out io.Writer, imp *channelImport) {
	if imp == nil {
		return
	}
	_, _ = fmt.Fprintf(out, "\nAlert channels: %d provider(s), %d unsupported\n", len(imp.Channels), len(imp.Unsupported))
	for _, c := range imp.Channels {
		mode := "disabled"
		if c.Enabled {
			mode = "enabled"
		}
		where := ""
		if c.Server != "" {
			where = ", " + cleanDetail(c.Server)
		}
		_, _ = fmt.Fprintf(out, "  - channel %s (from %q, %s%s, %s): %s\n", c.Name, c.SourceName, c.Kind, where, mode, c.State)
		if c.Detail != "" {
			_, _ = fmt.Fprintf(out, "      %s\n", c.Detail)
		}
	}
	for _, u := range imp.Unsupported {
		_, _ = fmt.Fprintf(out, "  - provider %q (%s): unsupported, %s\n", u.SourceName, u.SourceKind, u.Reason)
	}
}

func planChannelUndo(ctx context.Context, client *Client, r *journalResource) (action, reason string) {
	return planBackupUndo(ctx, client, r)
}
