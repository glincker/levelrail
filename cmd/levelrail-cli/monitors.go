package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runMonitors dispatches "monitors <verb>": experimental uptime checks, off unless the server sets APP_MONITORS_ENABLED.
func runMonitors(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	subs := map[string]apiCmd{"create": monitorsCreateCommand(), "list": monitorsListCommand(), "results": monitorsResultsCommand(), "update": monitorsUpdateCommand(),
		"delete": monitorsDeleteCommand(), "rotate-token": monitorsRotateCommand()}
	return dispatchSub(prog, "monitors", monitorsUsage, subs, args, stdout, stderr, lookupEnv)
}

func monitorsUsage(prog string) string {
	return fmt.Sprintf(`Usage (experimental, needs APP_MONITORS_ENABLED=true on the control plane):
  %[1]s monitors create --name N --kind http --url URL [--method GET] [--header NAME=VALUE]... [--expect 200-299,301] [--contains TEXT] [--follow-redirects] [--interval 60s] [--timeout 5s] [--status-link] [--link-confirm N] [--link-recovery 2m]
  %[1]s monitors create --name N --kind tcp --host HOST --port PORT [--interval 60s] [--timeout 5s] [--status-link]
  %[1]s monitors create --name N --kind heartbeat --interval 1h [--grace 10m] [--status-link]
  %[1]s monitors create --name N --kind tls --host HOST [--port 443] [--interval 24h] [--warn-days 21] [--crit-days 7] [--status-link]
  %[1]s monitors rotate-token <id>
  %[1]s monitors update <id> [--status-link=true|false] [--link-confirm N] [--link-recovery 2m]
  %[1]s monitors list
  %[1]s monitors results <id> [--window 24h] [--limit N]
  %[1]s monitors delete <id>

--status-link opens a status page incident on the monitor's component while it is confirmed
down and resolves it after recovery. It is off by default; --link-confirm and --link-recovery
override the APP_MONITOR_LINK_* defaults for this monitor.

A heartbeat monitor prints a secret ping path once, at create and at rotate-token. A cron job
calls it with curl: success is the plain URL, then /start before and /fail on error. The monitor
goes down when no ping arrives within --interval plus --grace (default from APP_MONITOR_HEARTBEAT_GRACE)
and up on the next ping. A tls monitor reports days left on the certificate: degraded under
--warn-days and down under --crit-days (defaults APP_MONITOR_TLS_WARN_DAYS and APP_MONITOR_TLS_CRIT_DAYS);
an invalid chain is reported as a failure. rotate-token replaces the heartbeat URL at once.

Header values are stored encrypted and are never shown again. Interval floors
and limits come from the APP_MONITOR_* settings on the control plane.
`, prog)
}

type headerFlags []string

func (h *headerFlags) String() string     { return strings.Join(*h, ",") }
func (h *headerFlags) Set(v string) error { *h = append(*h, v); return nil }

func parseExpect(s string) ([]apiclient.MonitorRange, error) {
	var out []apiclient.MonitorRange
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lo, hi, isRange := strings.Cut(part, "-")
		first, err := strconv.Atoi(lo)
		if err != nil {
			return nil, newValidationError("--expect %q is not a status or range", part)
		}
		last := first
		if isRange {
			if last, err = strconv.Atoi(hi); err != nil {
				return nil, newValidationError("--expect %q is not a status or range", part)
			}
		}
		out = append(out, apiclient.MonitorRange{Min: first, Max: last})
	}
	return out, nil
}

func monitorsCreateCommand() apiCmd {
	var name, kind, urlStr, method, expect, contains, host string
	var port int
	var follow, statusLink bool
	var linkConfirm, warnDays, critDays int
	var linkRecovery, grace time.Duration
	var headers headerFlags
	interval, timeout := 60*time.Second, 5*time.Second
	return apiCmd{label: "monitors create", usage: monitorsUsage,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&name, "name", "", "monitor name (required)")
			fs.StringVar(&kind, "kind", "http", "http, tcp, heartbeat or tls")
			fs.StringVar(&urlStr, "url", "", "http(s) URL to check (http kind)")
			fs.StringVar(&method, "method", "", "GET, HEAD, POST or PUT (default GET)")
			fs.Var(&headers, "header", "request header NAME=VALUE, repeatable; the value is stored encrypted")
			fs.StringVar(&expect, "expect", "", "accepted status codes or ranges, e.g. 200-299,301 (default 200-399)")
			fs.StringVar(&contains, "contains", "", "text that must appear in the start of the response body")
			fs.BoolVar(&follow, "follow-redirects", false, "follow redirects up to the server limit")
			fs.StringVar(&host, "host", "", "host to connect to (tcp and tls kinds)")
			fs.IntVar(&port, "port", 0, "port to connect to (tcp kind; tls defaults to 443)")
			fs.DurationVar(&interval, "interval", interval, "time between checks")
			fs.DurationVar(&timeout, "timeout", timeout, "per check timeout")
			fs.BoolVar(&statusLink, "status-link", false, "open and resolve a status page incident for this monitor")
			fs.IntVar(&linkConfirm, "link-confirm", 0, "failed checks before the incident opens (0 uses the server default)")
			fs.DurationVar(&linkRecovery, "link-recovery", 0, "time the monitor must stay up before the incident resolves (0 uses the server default)")
			fs.DurationVar(&grace, "grace", 0, "heartbeat kind: extra wait after the interval before down (0 uses the server default)")
			fs.IntVar(&warnDays, "warn-days", 0, "tls kind: degraded under this many days left (0 uses the server default)")
			fs.IntVar(&critDays, "crit-days", 0, "tls kind: down under this many days left (0 uses the server default)")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			if name == "" {
				return nil, nil, newValidationError("--name is required")
			}
			ranges, err := parseExpect(expect)
			if err != nil {
				return nil, nil, err
			}
			hdr := map[string]string{}
			for _, h := range headers {
				k, v, ok := strings.Cut(h, "=")
				if !ok || k == "" {
					return nil, nil, newValidationError("--header must look like NAME=VALUE")
				}
				hdr[k] = v
			}
			m := apiclient.Monitor{Name: name, Kind: kind, IntervalSeconds: int(interval / time.Second), TimeoutMS: int(timeout / time.Millisecond),
				URL: urlStr, Method: method, Headers: hdr, Expect: ranges, Contains: contains, FollowRedirects: follow, Host: host, Port: port,
				StatusLink: statusLink, LinkConfirmFailures: linkConfirm, LinkRecoverySeconds: int(linkRecovery / time.Second),
				GraceSeconds: int(grace / time.Second), WarnDays: warnDays, CritDays: critDays}
			created, err := c.CreateMonitor(ctx, m)
			return created, func(w io.Writer) {
				_, _ = fmt.Fprintf(w, "monitor %s created: %s (%s every %ds)\n", created.ID, created.Name, created.Kind, created.IntervalSeconds)
				printPingLine(w, c, created)
			}, err
		}}
}

func printPingLine(w io.Writer, c *Client, m apiclient.Monitor) {
	if m.PingPath == "" {
		return
	}
	_, _ = fmt.Fprintf(w, "ping (shown once): curl -fsS -m 10 --retry 3 %s%s\n", strings.TrimRight(c.BaseURL(), "/"), m.PingPath)
}

func monitorsRotateCommand() apiCmd {
	return apiCmd{label: "monitors rotate-token", usage: monitorsUsage, args: 1,
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			m, err := c.RotateMonitorToken(ctx, pos[0])
			return m, func(w io.Writer) {
				_, _ = fmt.Fprintf(w, "monitor %s token rotated, the old URL no longer works\n", m.ID)
				printPingLine(w, c, m)
			}, err
		}}
}

func monitorTarget(m apiclient.Monitor) string {
	if m.Kind == "heartbeat" {
		return "inbound ping"
	}
	if m.Kind == "tcp" || m.Kind == "tls" {
		return fmt.Sprintf("%s:%d", m.Host, m.Port)
	}
	return m.URL
}

func monitorsListCommand() apiCmd {
	return apiCmd{label: "monitors list", usage: monitorsUsage,
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			list, err := c.ListMonitors(ctx)
			return list, func(w io.Writer) {
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				_, _ = fmt.Fprintln(tw, "ID\tNAME\tKIND\tTARGET\tINTERVAL\tTIMEOUT")
				for _, m := range list {
					_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%ds\t%dms\n", m.ID, m.Name, m.Kind, monitorTarget(m), m.IntervalSeconds, m.TimeoutMS)
				}
				_ = tw.Flush()
			}, err
		}}
}

func monitorsResultsCommand() apiCmd {
	var window string
	var limit int
	return apiCmd{label: "monitors results", usage: monitorsUsage, args: 1,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&window, "window", "24h", "how far back to read, such as 6h or 168h")
			fs.IntVar(&limit, "limit", 20, "most raw results to list")
		},
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			res, err := c.MonitorResultsFor(ctx, pos[0], window, limit)
			return res, func(w io.Writer) {
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				_, _ = fmt.Fprintln(tw, "HOUR (UTC)\tCHECKS\tFAILED\tAVG MS\tMAX MS")
				for _, r := range res.Rollup {
					_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\n", time.Unix(r.Bucket, 0).UTC().Format("2006-01-02 15:04"), r.Total, r.Failed, r.AvgLatencyMS, r.MaxLatencyMS)
				}
				_ = tw.Flush()
				_, _ = fmt.Fprintln(w)
				tw = tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				_, _ = fmt.Fprintln(tw, "TIME (UTC)\tOK\tCODE\tMS\tREASON")
				for _, r := range res.Raw {
					_, _ = fmt.Fprintf(tw, "%s\t%t\t%d\t%d\t%s\n", time.Unix(r.At, 0).UTC().Format("2006-01-02 15:04:05"), r.OK, r.Code, r.LatencyMS, orDash(r.Reason))
				}
				_ = tw.Flush()
			}, err
		}}
}

func monitorsDeleteCommand() apiCmd {
	return apiCmd{label: "monitors delete", usage: monitorsUsage, args: 1,
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			err := c.DeleteMonitor(ctx, pos[0])
			return map[string]string{"deleted": pos[0]}, func(w io.Writer) { _, _ = fmt.Fprintf(w, "monitor %s deleted\n", pos[0]) }, err
		}}
}

func monitorsUpdateCommand() apiCmd {
	var statusLink bool
	var linkConfirm int
	var linkRecovery time.Duration
	return apiCmd{label: "monitors update", usage: monitorsUsage, args: 1,
		setup: func(fs *flag.FlagSet) {
			fs.BoolVar(&statusLink, "status-link", false, "open and resolve a status page incident for this monitor")
			fs.IntVar(&linkConfirm, "link-confirm", 0, "failed checks before the incident opens (0 uses the server default)")
			fs.DurationVar(&linkRecovery, "link-recovery", 0, "time the monitor must stay up before the incident resolves (0 uses the server default)")
		},
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			m, err := c.UpdateMonitorLink(ctx, pos[0], apiclient.MonitorLink{StatusLink: statusLink, LinkConfirmFailures: linkConfirm,
				LinkRecoverySeconds: int(linkRecovery / time.Second)})
			return m, func(w io.Writer) {
				_, _ = fmt.Fprintf(w, "monitor %s status link: %t\n", m.ID, m.StatusLink)
			}, err
		}}
}
