package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/glincker/levelrail/internal/apiclient"
)

func runDatabasesRedis(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "info" && args[0] != "slowlog" && args[0] != "clients") {
		_, _ = fmt.Fprintf(stderr, "usage: %s databases redis info|slowlog|clients <name> [--json]\n", prog)
		return 2
	}
	sub := args[0]
	c, code, ok := parseDBAccess(prog, "databases redis "+sub, "databases redis "+sub+" <name> [flags]", args[1:], 1, "a database name", stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	name := c.rest[0]
	ctx := context.Background()
	switch sub {
	case "info":
		v, err := c.client.GetRedisInfo(ctx, name)
		if err != nil {
			return c.fail(fmt.Errorf("redis info of database %q: %w", name, err))
		}
		return c.result(v, func() { printRedisInfo(stdout, v) })
	case "slowlog":
		v, err := c.client.GetRedisSlowlog(ctx, name)
		if err != nil {
			return c.fail(fmt.Errorf("redis slowlog of database %q: %w", name, err))
		}
		return c.result(v, func() { printRedisSlowlog(stdout, v) })
	case "clients":
		v, err := c.client.GetRedisClients(ctx, name)
		if err != nil {
			return c.fail(fmt.Errorf("redis clients of database %q: %w", name, err))
		}
		return c.result(v, func() { printRedisClients(stdout, v) })
	}
	return 2
}

func printRedisInfo(out io.Writer, v apiclient.RedisInfo) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	for _, s := range v.Sections {
		_, _ = fmt.Fprintf(tw, "\n[%s]\n", safeTerminalText(s.Name))
		for _, f := range s.Fields {
			_, _ = fmt.Fprintf(tw, "%s\t%s\n", safeTerminalText(f.Key), safeTerminalText(f.Value))
		}
	}
	_ = tw.Flush()
}

func printRedisSlowlog(out io.Writer, v apiclient.RedisSlowlog) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tTIME\tMICROSECONDS\tCOMMAND\tARGS\tCLIENT")
	for _, e := range v.Entries {
		_, _ = fmt.Fprintf(tw, "%d\t%d\t%d\t%s\t%d\t%s\n", e.ID, e.Time, e.DurationUS, safeTerminalText(e.Command), e.ArgCount, safeTerminalText(e.Client))
	}
	_ = tw.Flush()
}

func printRedisClients(out io.Writer, v apiclient.RedisClients) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tADDR\tAGE\tIDLE\tFLAGS\tDB\tCMD\tUSER")
	for _, cl := range v.Clients {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\t%s\t%s\t%s\n", safeTerminalText(cl.ID), safeTerminalText(cl.Addr), cl.Age, cl.Idle, safeTerminalText(cl.Flags), safeTerminalText(cl.DB), safeTerminalText(cl.Cmd), safeTerminalText(cl.User))
	}
	_ = tw.Flush()
}
