package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runStatusPage dispatches "status-page <verb>": the operator side of the
// opt-in public status page.
func runStatusPage(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	top := map[string]apiCmd{"get": statusGetCommand(), "set": statusSetCommand(), "preview": statusPreviewCommand()}
	if len(args) > 0 {
		switch args[0] {
		case "components":
			subs := map[string]apiCmd{"list": statusComponentsListCommand(), "add": statusComponentAddCommand(),
				"delete": statusDeleteCommand("component", func(ctx context.Context, c *Client, id string) error { return c.DeleteStatusComponent(ctx, id) })}
			return dispatchSub(prog, "status-page components", statusPageUsage, subs, args[1:], stdout, stderr, lookupEnv)
		case "incidents":
			subs := map[string]apiCmd{"list": statusIncidentsListCommand(), "create": statusIncidentCreateCommand(), "update": statusIncidentUpdateCommand(),
				"delete": statusDeleteCommand("incident", func(ctx context.Context, c *Client, id string) error { return c.DeleteStatusIncident(ctx, id) })}
			return dispatchSub(prog, "status-page incidents", statusPageUsage, subs, args[1:], stdout, stderr, lookupEnv)
		}
	}
	return dispatchSub(prog, "status-page", statusPageUsage, top, args, stdout, stderr, lookupEnv)
}

func statusPageUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s status-page get
  %[1]s status-page set [--enable|--disable] [--title T] [--description D] [--domain HOST]
  %[1]s status-page preview
  %[1]s status-page components list
  %[1]s status-page components add --kind app|domain|check --target T --name PUBLIC_NAME
  %[1]s status-page components delete <id>
  %[1]s status-page incidents list
  %[1]s status-page incidents create --title T [--kind incident|maintenance] [--impact none|minor|major|critical] [--body TEXT] [--component ID]... [--starts TIME --ends TIME]
  %[1]s status-page incidents update <id> --status S --body TEXT
  %[1]s status-page incidents delete <id>

The public page is off until "set --enable". Only the public component
names, statuses and operator-written announcements are ever published;
targets (app names, hostnames, check URLs) stay private.
`, prog)
}

func statusGetCommand() apiCmd {
	return apiCmd{label: "status-page get", usage: statusPageUsage,
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			s, err := c.GetStatusPage(ctx)
			return s, func(w io.Writer) { printStatusSettings(w, s) }, err
		}}
}

func statusSetCommand() apiCmd {
	var (
		enable, disable     bool
		title, desc, domain string
		fsRef               *flag.FlagSet
	)
	return apiCmd{label: "status-page set", usage: statusPageUsage,
		setup: func(fs *flag.FlagSet) {
			fsRef = fs
			fs.BoolVar(&enable, "enable", false, "publish the page")
			fs.BoolVar(&disable, "disable", false, "unpublish the page")
			fs.StringVar(&title, "title", "", "page title")
			fs.StringVar(&desc, "description", "", "short description under the title")
			fs.StringVar(&domain, "domain", "", "serve the page on this hostname (the ingress must route it to the control plane)")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			if enable && disable {
				return nil, nil, newValidationError("--enable and --disable are mutually exclusive")
			}
			cur, err := c.GetStatusPage(ctx)
			if err != nil {
				return nil, nil, err
			}
			fsRef.Visit(func(f *flag.Flag) {
				switch f.Name {
				case "title":
					cur.Title = title
				case "description":
					cur.Description = desc
				case "domain":
					cur.CustomDomain = domain
				}
			})
			if enable {
				cur.Enabled = true
			}
			if disable {
				cur.Enabled = false
			}
			s, err := c.PutStatusPage(ctx, cur)
			return s, func(w io.Writer) { printStatusSettings(w, s) }, err
		}}
}

func printStatusSettings(w io.Writer, s apiclient.StatusPageSettings) {
	_, _ = fmt.Fprintf(w, "enabled: %t\ntitle: %s\ndescription: %s\ncustom domain: %s\npublic path: %s\n", s.Enabled, orDash(s.Title), orDash(s.Description), orDash(s.CustomDomain), orDash(s.PublicPath))
}

func statusPreviewCommand() apiCmd {
	return apiCmd{label: "status-page preview", usage: statusPageUsage,
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			v, err := c.StatusPagePreview(ctx)
			return v, func(w io.Writer) {
				_, _ = fmt.Fprintf(w, "%s: %s\n", v.Title, v.StatusText)
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				for _, comp := range v.Components {
					up := "no data"
					if comp.Uptime90 != nil {
						up = fmt.Sprintf("%.2f%%", *comp.Uptime90)
					}
					_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", comp.Name, comp.Status, up)
				}
				_ = tw.Flush()
			}, err
		}}
}

func statusComponentsListCommand() apiCmd {
	return apiCmd{label: "status-page components list", usage: statusPageUsage,
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			list, err := c.ListStatusComponents(ctx)
			return list, func(w io.Writer) {
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				_, _ = fmt.Fprintln(tw, "ID\tKIND\tTARGET\tPUBLIC NAME")
				for _, comp := range list {
					_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", comp.ID, comp.Kind, comp.Target, comp.DisplayName)
				}
				_ = tw.Flush()
			}, err
		}}
}

func statusComponentAddCommand() apiCmd {
	var kind, target, name string
	return apiCmd{label: "status-page components add", usage: statusPageUsage,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&kind, "kind", "", "app, domain or check (required)")
			fs.StringVar(&target, "target", "", "app name, hostname or http(s) URL (required, never published)")
			fs.StringVar(&name, "name", "", "public display name (required)")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			if kind == "" || target == "" || name == "" {
				return nil, nil, newValidationError("--kind, --target and --name are required")
			}
			comp, err := c.CreateStatusComponent(ctx, apiclient.StatusComponent{Kind: kind, Target: target, DisplayName: name})
			return comp, func(w io.Writer) { _, _ = fmt.Fprintf(w, "component %s added as %q\n", comp.ID, comp.DisplayName) }, err
		}}
}

func statusDeleteCommand(noun string, del func(context.Context, *Client, string) error) apiCmd {
	return apiCmd{label: "status-page " + noun + "s delete", usage: statusPageUsage, args: 1,
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			err := del(ctx, c, pos[0])
			return map[string]string{"deleted": pos[0]}, func(w io.Writer) { _, _ = fmt.Fprintf(w, "%s %s deleted\n", noun, pos[0]) }, err
		}}
}

func statusIncidentsListCommand() apiCmd {
	return apiCmd{label: "status-page incidents list", usage: statusPageUsage,
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			list, err := c.ListStatusIncidents(ctx)
			return list, func(w io.Writer) {
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				_, _ = fmt.Fprintln(tw, "ID\tKIND\tSTATUS\tIMPACT\tTITLE")
				for _, i := range list {
					_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", i.ID, i.Kind, i.Status, i.Impact, i.Title)
				}
				_ = tw.Flush()
			}, err
		}}
}

func statusIncidentCreateCommand() apiCmd {
	var (
		kind, title, impact, body, starts, ends string
		components                              stringList
	)
	return apiCmd{label: "status-page incidents create", usage: statusPageUsage,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&kind, "kind", "incident", "incident or maintenance")
			fs.StringVar(&title, "title", "", "headline (required)")
			fs.StringVar(&impact, "impact", "none", "none, minor, major or critical (colors affected components)")
			fs.StringVar(&body, "body", "", "first update text (markdown-lite: **bold**, `code`, - lists, [text](https://url))")
			fs.Var(&components, "component", "affected component ID (repeatable)")
			fs.StringVar(&starts, "starts", "", "RFC 3339 start (maintenance requires it)")
			fs.StringVar(&ends, "ends", "", "RFC 3339 end (maintenance requires it)")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			if title == "" {
				return nil, nil, newValidationError("--title is required")
			}
			inc := apiclient.StatusIncident{Kind: kind, Title: title, Impact: impact, Body: body, ComponentIDs: components}
			var err error
			if inc.StartsAt, err = parseOptionalTime(starts); err != nil {
				return nil, nil, newValidationError("--starts: %v", err)
			}
			if inc.EndsAt, err = parseOptionalTime(ends); err != nil {
				return nil, nil, newValidationError("--ends: %v", err)
			}
			created, err := c.CreateStatusIncident(ctx, inc)
			return created, func(w io.Writer) {
				_, _ = fmt.Fprintf(w, "%s %s created (%s)\n", created.Kind, created.ID, created.Status)
			}, err
		}}
}

func parseOptionalTime(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func statusIncidentUpdateCommand() apiCmd {
	var status, body string
	return apiCmd{label: "status-page incidents update", usage: statusPageUsage, args: 1,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&status, "status", "", "investigating, identified, monitoring, resolved (incident) or scheduled, in_progress, completed (maintenance)")
			fs.StringVar(&body, "body", "", "update text (required)")
		},
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			if status == "" || body == "" {
				return nil, nil, newValidationError("--status and --body are required")
			}
			inc, err := c.PostStatusIncidentUpdate(ctx, pos[0], apiclient.StatusUpdate{Status: status, Body: body})
			return inc, func(w io.Writer) { _, _ = fmt.Fprintf(w, "%s is now %s\n", inc.ID, inc.Status) }, err
		}}
}
