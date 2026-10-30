package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func statusWebhookSubs() map[string]apiCmd {
	return map[string]apiCmd{
		"list":   statusWebhooksListCommand(),
		"add":    statusWebhookAddCommand(),
		"rotate": statusWebhookActionCommand("rotate", "signing secret rotated, the new secret is shown once"),
		"pause":  statusWebhookActionCommand("pause", "paused, no events are sent"),
		"resume": statusWebhookActionCommand("resume", "resumed"),
		"delete": statusDeleteCommand("webhook", func(ctx context.Context, c *Client, id string) error { return c.DeleteStatusWebhook(ctx, id) }),
		"test":   statusWebhookTestCommand(),
	}
}

func statusWebhooksListCommand() apiCmd {
	return apiCmd{label: "status-page webhooks list", usage: statusPageUsage,
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			list, err := c.ListStatusWebhooks(ctx)
			return list, func(w io.Writer) {
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				_, _ = fmt.Fprintln(tw, "ID\tSTATE\tFAILURES\tLABEL\tURL\tEVENTS")
				for _, h := range list {
					state := "active"
					if h.Paused {
						state = "paused"
					}
					events := "all"
					if len(h.Events) > 0 {
						events = strings.Join(h.Events, ",")
					}
					_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\n", h.ID, state, h.ConsecutiveFailures, orDash(h.Label), h.URL, events)
				}
				_ = tw.Flush()
			}, err
		}}
}

func statusWebhookAddCommand() apiCmd {
	var (
		url, label string
		events     stringList
	)
	return apiCmd{label: "status-page webhooks add", usage: statusPageUsage,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&url, "url", "", "https URL that receives the signed events (required)")
			fs.StringVar(&label, "label", "", "short name for this subscriber")
			fs.Var(&events, "event", "event to send, repeatable: incident_opened, incident_updated, incident_resolved, component_changed, maintenance_scheduled, maintenance_updated, maintenance_completed (default: all)")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			if url == "" {
				return nil, nil, newValidationError("--url is required")
			}
			h, err := c.CreateStatusWebhook(ctx, apiclientWebhook(url, label, events))
			return h, func(w io.Writer) {
				_, _ = fmt.Fprintf(w, "webhook %s added\nsigning secret (shown once): %s\n", h.ID, h.SigningSecret)
			}, err
		}}
}

func statusWebhookActionCommand(action, done string) apiCmd {
	return apiCmd{label: "status-page webhooks " + action, usage: statusPageUsage, args: 1,
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			h, err := c.StatusWebhookAction(ctx, pos[0], action)
			return h, func(w io.Writer) {
				_, _ = fmt.Fprintf(w, "webhook %s: %s\n", h.ID, done)
				if h.SigningSecret != "" {
					_, _ = fmt.Fprintf(w, "signing secret (shown once): %s\n", h.SigningSecret)
				}
			}, err
		}}
}

func statusWebhookTestCommand() apiCmd {
	return apiCmd{label: "status-page webhooks test", usage: statusPageUsage, args: 1,
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			r, err := c.TestStatusWebhook(ctx, pos[0])
			return r, func(w io.Writer) {
				if r.Delivered {
					_, _ = fmt.Fprintf(w, "ping delivered, receiver answered HTTP %d\n", r.StatusCode)
					return
				}
				_, _ = fmt.Fprintf(w, "ping failed: %s\n", r.Error)
			}, err
		}}
}

func apiclientWebhook(url, label string, events []string) apiclient.StatusWebhook {
	return apiclient.StatusWebhook{URL: url, Label: label, Events: events}
}
