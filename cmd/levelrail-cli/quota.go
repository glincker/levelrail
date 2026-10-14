package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func quotaUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s quota show <team|project> <id>            limits and current usage
  %[1]s quota set <team|project> <id> [flags]     replace the limits (admins); 0 means unlimited
  %[1]s quota clear <team|project> <id>           remove every limit (admins)

Flags for set: --max-apps N --max-databases N --max-memory-mb N --max-cpu F --max-previews N
`, prog)
}

func runQuota(prog string, args []string, stdout, stderr io.Writer, lookupEnv lookupFn) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, quotaUsage(prog))
		return exitUsage
	}
	verb, rest := args[0], args[1:]
	if verb == "-h" || verb == "--help" || verb == "help" {
		_, _ = fmt.Fprint(stdout, quotaUsage(prog))
		return exitOK
	}
	if verb != "show" && verb != "set" && verb != "clear" {
		_, _ = fmt.Fprintf(stderr, "%s: unknown quota subcommand %q\n\n%s", prog, verb, quotaUsage(prog))
		return exitUsage
	}
	var limits apiclient.QuotaLimits
	c, code, ok := parseTeamsCmd(prog, "quota "+verb, "quota "+verb+" <team|project> <id> [flags]", 2, rest, stderr, lookupEnv, func(fs *flag.FlagSet) {
		if verb != "set" {
			return
		}
		fs.Int64Var(&limits.MaxApps, "max-apps", 0, "most apps (0 unlimited)")
		fs.Int64Var(&limits.MaxDatabases, "max-databases", 0, "most databases (0 unlimited)")
		fs.Int64Var(&limits.MaxMemoryMB, "max-memory-mb", 0, "total memory reservation in MB (0 unlimited)")
		fs.Float64Var(&limits.MaxCPU, "max-cpu", 0, "total CPU reservation in cores (0 unlimited)")
		fs.Int64Var(&limits.MaxPreviews, "max-previews", 0, "most live preview environments (0 unlimited)")
	})
	if !ok {
		return code
	}
	scope, id := c.args[0], c.args[1]
	if scope != "team" && scope != "project" {
		_, _ = fmt.Fprintf(stderr, "%s: quota scope must be team or project, got %q\n", prog, scope)
		return exitUsage
	}
	ctx := context.Background()
	var res apiclient.QuotaResource
	var err error
	switch verb {
	case "show":
		res, err = c.client.GetQuota(ctx, scope, id)
	case "set":
		res, err = c.client.SetQuota(ctx, scope, id, limits)
	case "clear":
		res, err = c.client.ClearQuota(ctx, scope, id)
	}
	return c.finish(stdout, stderr, res, err, func() { printQuota(stdout, res) })
}

func printQuota(out io.Writer, q apiclient.QuotaResource) {
	_, _ = fmt.Fprintf(out, "%s %s (%s)\n", q.Scope, q.Name, q.ScopeID)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "RESOURCE\tUSED\tLIMIT")
	cpu := func(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
	limit := func(n string, unlimited bool) string {
		if unlimited {
			return "unlimited"
		}
		return n
	}
	e, u := q.Effective, q.Usage
	_, _ = fmt.Fprintf(tw, "apps\t%d\t%s\n", u.Apps, limit(strconv.FormatInt(e.MaxApps, 10), e.MaxApps == 0))
	_, _ = fmt.Fprintf(tw, "databases\t%d\t%s\n", u.Databases, limit(strconv.FormatInt(e.MaxDatabases, 10), e.MaxDatabases == 0))
	_, _ = fmt.Fprintf(tw, "memory (MB)\t%d\t%s\n", u.MemoryMB, limit(strconv.FormatInt(e.MaxMemoryMB, 10), e.MaxMemoryMB == 0))
	_, _ = fmt.Fprintf(tw, "cpu (cores)\t%s\t%s\n", cpu(u.CPU), limit(cpu(e.MaxCPU), e.MaxCPU == 0))
	_, _ = fmt.Fprintf(tw, "previews\t%d\t%s\n", u.Previews, limit(strconv.FormatInt(e.MaxPreviews, 10), e.MaxPreviews == 0))
	_ = tw.Flush()
}
