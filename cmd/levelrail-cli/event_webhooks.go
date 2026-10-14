package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func eventWebhooksUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s event-webhooks list [flags]                        list webhooks with delivery counts
  %[1]s event-webhooks create --name N --url U [flags]     add a webhook, prints the signing secret once
  %[1]s event-webhooks test <id> [flags]                   send a signed probe and report the result
  %[1]s event-webhooks deliveries <id> [flags]             show the delivery log (state, attempts, last status)
  %[1]s event-webhooks replay <delivery-id> [flags]        re-send a failed or dead-lettered delivery
  %[1]s event-webhooks enable|disable <id> [flags]         pause or resume delivery
  %[1]s event-webhooks rotate-secret <id> [flags]          issue a new signing secret, printed once
  %[1]s event-webhooks delete <id> [flags]                 remove a webhook and its delivery log
`, prog)
}

// runEventWebhooks handles "event-webhooks ...", the outbound event
// subscription surface.
func runEventWebhooks(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, eventWebhooksUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, eventWebhooksUsage(prog))
		return exitOK
	case "list":
		return runEventWebhookList(prog, rest, stdout, stderr, lookupEnv)
	case "create":
		return runEventWebhookCreate(prog, rest, stdout, stderr, lookupEnv)
	case "test":
		return runEventWebhookTest(prog, rest, stdout, stderr, lookupEnv)
	case "deliveries":
		return runEventWebhookDeliveries(prog, rest, stdout, stderr, lookupEnv)
	case "replay":
		return runEventWebhookReplay(prog, rest, stdout, stderr, lookupEnv)
	case "enable", "disable":
		return runEventWebhookToggle(prog, args[0], rest, stdout, stderr, lookupEnv)
	case "rotate-secret":
		return runEventWebhookRotate(prog, rest, stdout, stderr, lookupEnv)
	case "delete":
		return runEventWebhookDelete(prog, rest, stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown event-webhooks subcommand %q\n\n%s", prog, args[0], eventWebhooksUsage(prog))
	return exitUsage
}

func eventWebhookIDCall(prog, label, what string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) (cliCall, string, int, bool) {
	usage := fmt.Sprintf("Usage:\n  %s %s <%s> [flags]\n%s", prog, label, what, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return cliCall{}, "", code, false
	}
	id, ok := requireOneArg(c.fs, stderr, prog, label, what)
	if !ok {
		return cliCall{}, "", exitUsage, false
	}
	return c, id, exitOK, true
}

func runEventWebhookList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "event-webhooks list", eventWebhooksUsage(prog)+commonFlagsHelp, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	hooks, err := c.client.ListEventWebhooks(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("list event webhooks: %w", err))
	}
	return c.render(hooks, func() {
		if len(hooks) == 0 {
			_, _ = fmt.Fprintln(stdout, "no event webhooks")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tNAME\tENABLED\tDELIVERED\tPENDING\tDEAD\tLAST STATUS\tURL")
		for _, h := range hooks {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%t\t%d\t%d\t%d\t%d\t%s\n", h.ID, h.Name, h.Enabled, h.Delivered, h.Pending, h.Dead, h.LastStatus, h.URL)
		}
		_ = tw.Flush()
	})
}

func runEventWebhookCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var name, url string
	var disabled bool
	var events csvListFlag
	usage := fmt.Sprintf(`Usage:
  %[1]s event-webhooks create --name NAME --url URL [flags]

Flags:
  --name string     webhook name (required)
  --url string      https endpoint that receives signed POSTs (required)
  --event string    event type or family such as apps.* (comma separated or repeated), default all
  --disabled        create the webhook paused
%[2]s`, prog, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, "event-webhooks create", usage, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&name, "name", "", "webhook name")
		fs.StringVar(&url, "url", "", "endpoint URL")
		fs.Var(&events, "event", "event types to deliver")
		fs.BoolVar(&disabled, "disabled", false, "create paused")
	})
	if !ok {
		return code
	}
	if name == "" || url == "" {
		return c.invalid("--name and --url are required")
	}
	enabled := !disabled
	created, err := c.client.CreateEventWebhook(context.Background(), apiclient.CreateEventWebhookRequest{
		Name: name, URL: url, Enabled: &enabled, Events: events,
	})
	if err != nil {
		return c.fail(fmt.Errorf("create event webhook %q: %w", name, err))
	}
	return c.render(created, func() {
		_, _ = fmt.Fprintf(stdout, "event webhook %q created (id %s)\nsigning secret (shown once): %s\n", created.Name, created.ID, created.SigningSecret)
	})
}

func runEventWebhookTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := eventWebhookIDCall(prog, "event-webhooks test", "webhook id", args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	res, err := c.client.TestEventWebhook(context.Background(), id)
	if err != nil {
		return c.fail(fmt.Errorf("test event webhook %q: %w", id, err))
	}
	status := c.render(res, func() {
		if res.OK {
			_, _ = fmt.Fprintf(stdout, "webhook %s accepted the test probe (HTTP %d)\n", id, res.Status)
			return
		}
		_, _ = fmt.Fprintf(stdout, "webhook %s failed: %s\n", id, res.Error)
	})
	if status == exitOK && !res.OK {
		return exitCheckFailed
	}
	return status
}

func runEventWebhookDeliveries(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := eventWebhookIDCall(prog, "event-webhooks deliveries", "webhook id", args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	ds, err := c.client.ListEventWebhookDeliveries(context.Background(), id)
	if err != nil {
		return c.fail(fmt.Errorf("list deliveries for %q: %w", id, err))
	}
	return c.render(ds, func() {
		if len(ds) == 0 {
			_, _ = fmt.Fprintln(stdout, "no deliveries yet")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tEVENT\tSTATE\tATTEMPTS\tHTTP\tWHEN\tERROR")
		for _, d := range ds {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%s\t%s\n", d.ID, d.EventType, d.State, d.Attempts, d.ResponseStatus, d.UpdatedAt, dash(d.LastError))
		}
		_ = tw.Flush()
	})
}

func runEventWebhookReplay(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := eventWebhookIDCall(prog, "event-webhooks replay", "delivery id", args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	d, err := c.client.ReplayEventWebhookDelivery(context.Background(), id)
	if err != nil {
		return c.fail(fmt.Errorf("replay delivery %q: %w", id, err))
	}
	return c.render(d, func() { _, _ = fmt.Fprintf(stdout, "delivery %s queued for another attempt\n", d.ID) })
}

func runEventWebhookToggle(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := eventWebhookIDCall(prog, "event-webhooks "+verb, "webhook id", args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	enabled := verb == "enable"
	h, err := c.client.UpdateEventWebhook(context.Background(), id, apiclient.UpdateEventWebhookRequest{Enabled: &enabled})
	if err != nil {
		return c.fail(fmt.Errorf("%s event webhook %q: %w", verb, id, err))
	}
	return c.render(h, func() { _, _ = fmt.Fprintf(stdout, "event webhook %q %sd\n", h.Name, verb) })
}

func runEventWebhookRotate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := eventWebhookIDCall(prog, "event-webhooks rotate-secret", "webhook id", args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	h, err := c.client.RotateEventWebhookSecret(context.Background(), id)
	if err != nil {
		return c.fail(fmt.Errorf("rotate secret for %q: %w", id, err))
	}
	return c.render(h, func() {
		_, _ = fmt.Fprintf(stdout, "new signing secret for %q (shown once): %s\n", h.Name, h.SigningSecret)
	})
}

func runEventWebhookDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := eventWebhookIDCall(prog, "event-webhooks delete", "webhook id", args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	if err := c.client.DeleteEventWebhook(context.Background(), id); err != nil {
		return c.fail(fmt.Errorf("delete event webhook %q: %w", id, err))
	}
	return c.render(map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "event webhook %q deleted\n", id)
	})
}
