package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/glincker/levelrail/internal/apiclient"
)

const buildersWhatNext = "Next: run the command above on the machine that will build, then `%[1]s builder doctor` and `%[1]s builders test <name>` here."

func buildersUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s builders list [flags]                 builders, tier, platform, slots, last seen and recent build time
  %[1]s builders add [--name N] [flags]       mint a personal builder join token and print the join command
  %[1]s builders show <name> [flags]          one builder with its last 10 builds
  %[1]s builders pause <name> [flags]         stop new builds, running ones finish
  %[1]s builders resume <name> [flags]        accept new builds again
  %[1]s builders test <name> [flags]          run a tiny probe build (no app data) and report latency and arch
  %[1]s builders remove <name> [flags]        remove the node and revoke its certificate

Builders are build-only nodes (laptops, home machines, cloud instances). Add one with
"builders add", then run the printed "builder join" line on that machine.
`, prog)
}

func runBuilders(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprint(stdout, buildersUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	sub := args[0]
	switch sub {
	case "list", "add", "show", "pause", "resume", "test", "remove":
	default:
		_, _ = fmt.Fprintf(stderr, "%s builders: unknown command %q\n\n%s", prog, sub, buildersUsage(prog))
		return exitUsage
	}
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "builders "+sub, "print the result as JSON", stderr)
	var name string
	if sub == "add" {
		fs.StringVar(&name, "name", "", "name the builder enrolls with (default: its hostname)")
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, buildersUsage(prog)) }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	want := 1
	if sub == "list" || sub == "add" {
		want = 0
	}
	rest, ok := requireArgs(fs, stderr, prog, "builders "+sub, "a builder name", want)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	ctx := context.Background()
	var data any
	var table func()
	var err error
	switch sub {
	case "list":
		var res apiclient.BuildersResponse
		if res, err = client.ListBuilders(ctx); err == nil {
			data, table = res, func() { renderBuildersTable(stdout, res.Builders) }
		}
	case "add":
		data, table, err = buildersAdd(ctx, prog, client, name, stdout)
	case "show":
		var res apiclient.BuilderDetailResource
		if res, err = client.GetBuilder(ctx, rest[0]); err == nil {
			data, table = res, func() { renderBuilderDetail(stdout, res) }
		}
	case "pause", "resume":
		var res apiclient.BuilderResource
		if sub == "pause" {
			res, err = client.PauseBuilder(ctx, rest[0])
		} else {
			res, err = client.ResumeBuilder(ctx, rest[0])
		}
		if err == nil {
			data, table = res, func() { _, _ = fmt.Fprintf(stdout, "builder %q is now %s\n", res.Name, res.Status) }
		}
	case "test":
		var res apiclient.BuilderTestResponse
		if res, err = client.TestBuilder(ctx, rest[0]); err == nil {
			data, table = res, func() { renderBuilderTest(stdout, res) }
		}
	case "remove":
		var b apiclient.BuilderDetailResource
		if b, err = client.GetBuilder(ctx, rest[0]); err == nil {
			if err = client.DeleteNode(ctx, b.ID); err == nil {
				data = map[string]string{"id": b.ID, "name": b.Name, "removed": "true"}
				table = func() { _, _ = fmt.Fprintf(stdout, "builder %q removed; its certificate is revoked\n", b.Name) }
			}
		}
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("builders %s: %w", sub, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, data, table); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if t, isTest := data.(apiclient.BuilderTestResponse); isTest && !t.OK {
		return exitCheckFailed
	}
	return exitOK
}

func buildersAdd(ctx context.Context, prog string, client *Client, name string, stdout io.Writer) (any, func(), error) {
	created, err := client.CreateNodeJoinTokenFor(ctx, apiclient.CreateNodeJoinTokenRequest{Builder: true, BuildTrust: "personal", NodeName: name})
	if err != nil {
		return nil, nil, err
	}
	line := builderJoinLine(prog, client.BaseURL(), name, created)
	out := map[string]any{"token": created.Token, "expires_at": created.ExpiresAt, "join_command": line}
	return out, func() {
		_, _ = fmt.Fprintf(stdout, "Run this on the machine that will build (the token is shown once, expires %s):\n\n  %s\n\n", created.ExpiresAt.Format(time.RFC3339), line)
		_, _ = fmt.Fprintf(stdout, buildersWhatNext+"\n", prog)
	}, nil
}

// builderJoinLine is the exact one-line command a laptop runs to enrol.
func builderJoinLine(prog, server, name string, c apiclient.CreateNodeJoinTokenResponse) string {
	parts := []string{prog, "builder", "join", "--server", server, "--token", c.Token}
	if name != "" {
		parts = append(parts, "--name", name)
	}
	if c.ControlPlaneAddr != "" {
		parts = append(parts, "--agent-addr", c.ControlPlaneAddr)
	}
	if c.CAFingerprint != "" {
		parts = append(parts, "--ca-fingerprint", c.CAFingerprint)
	}
	return strings.Join(parts, " ")
}

func fmtBuildMS(ms int64) string {
	if ms <= 0 {
		return "-"
	}
	return (time.Duration(ms) * time.Millisecond).Round(100 * time.Millisecond).String()
}

func fmtSeen(t *time.Time) string {
	if t == nil {
		return "never"
	}
	return time.Since(*t).Round(time.Second).String() + " ago"
}

func fmtPlatform(b apiclient.BuilderResource) string {
	if b.OS == "" && b.Arch == "" {
		return "-"
	}
	return b.OS + "/" + b.Arch
}

func renderBuildersTable(w io.Writer, rows []apiclient.BuilderResource) {
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(w, "No builders yet. Add one with `builders add`.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tSTATUS\tTIER\tPLATFORM\tBUILDS\tLAST SEEN\tAVG BUILD")
	for _, b := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d/%d\t%s\t%s\n", b.Name, b.Status, b.Tier, fmtPlatform(b), b.ActiveBuilds, b.SlotsTotal, fmtSeen(b.LastSeenAt), fmtBuildMS(b.AvgBuildMS))
	}
	_ = tw.Flush()
}

func renderBuilderDetail(w io.Writer, d apiclient.BuilderDetailResource) {
	_, _ = fmt.Fprintf(w, "%s (%s)\nstatus: %s  tier: %s  platform: %s  agent: %s\nslots: %d/%d  waiting: %d  pressure: %s  last seen: %s\n",
		d.Name, d.ID, d.Status, d.Tier, fmtPlatform(d.BuilderResource), orDash(d.AgentVersion), d.ActiveBuilds, d.SlotsTotal, d.Waiting, d.Pressure, fmtSeen(d.LastSeenAt))
	if len(d.Builds) == 0 {
		_, _ = fmt.Fprintln(w, "\nNo builds on this builder yet.")
		return
	}
	_, _ = fmt.Fprintln(w, "\nLast builds")
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "APP\tRESULT\tDURATION\tSTARTED")
	for _, r := range d.Builds {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.App, r.Result, fmtBuildMS(r.DurationMS), r.StartedAt.Format(time.RFC3339))
	}
	_ = tw.Flush()
}

func renderBuilderTest(w io.Writer, t apiclient.BuilderTestResponse) {
	if !t.OK {
		_, _ = fmt.Fprintf(w, "FAIL %s: %s\n", t.Name, orDash(t.Error))
		return
	}
	_, _ = fmt.Fprintf(w, "PASS %s: %dms, %s/%s, buildkit %s, free disk %s\n", t.Name, t.LatencyMS, orDash(t.OS), orDash(t.Arch), orDash(t.BuildKit), fmtGiB(t.DiskFreeBytes))
}

func fmtGiB(b int64) string {
	if b <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30))
}
