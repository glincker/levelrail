package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runStorageDestinations(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, storageUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "ls", "list":
		return runStorageList(prog, rest, stdout, stderr, lookupEnv)
	case "add":
		return runStorageAdd(prog, rest, stdout, stderr, lookupEnv)
	case "test":
		return runStorageTest(prog, rest, stdout, stderr, lookupEnv)
	case "default":
		return runStorageDefault(prog, rest, stdout, stderr, lookupEnv)
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, storageUsage(prog))
		return exitOK
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown storage destinations subcommand %q\n\n%s", prog, args[0], storageUsage(prog))
		return exitUsage
	}
}

func runStorageDefault(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	action := ""
	if len(args) > 0 && (args[0] == "set" || args[0] == "clear") {
		action, args = args[0], args[1:]
	}
	c, code, ok := parseCLICall(prog, "storage destinations default", storageUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	ctx := context.Background()
	switch action {
	case "set":
		pos := c.fs.Args()
		if len(pos) != 2 {
			return c.invalid("usage: storage destinations default set <class> <destination id>")
		}
		res, err := c.client.SetStorageDefault(ctx, pos[0], pos[1])
		if err != nil {
			return c.fail(fmt.Errorf("set default for %q: %w", pos[0], err))
		}
		return c.render(res, func() {
			_, _ = fmt.Fprintf(stdout, "%s now go to %s\n", res.DataClass, res.TargetID)
		})
	case "clear":
		pos := c.fs.Args()
		if len(pos) != 1 {
			return c.invalid("usage: storage destinations default clear <class>")
		}
		if err := c.client.ClearStorageDefault(ctx, pos[0]); err != nil {
			return c.fail(fmt.Errorf("clear default for %q: %w", pos[0], err))
		}
		return c.render(map[string]bool{"cleared": true}, func() {
			_, _ = fmt.Fprintf(stdout, "default for %s cleared\n", pos[0])
		})
	}
	defaults, err := c.client.ListStorageDefaults(ctx)
	if err != nil {
		return c.fail(fmt.Errorf("list storage defaults: %w", err))
	}
	return c.render(defaults, func() {
		byClass := map[string]apiclient.StorageDefault{}
		for _, d := range defaults.Defaults {
			byClass[d.DataClass] = d
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "DATA CLASS\tDESTINATION")
		for _, class := range defaults.Classes {
			target := "-"
			if d, ok := byClass[class]; ok {
				target = d.TargetID
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\n", class, target)
		}
		_ = tw.Flush()
	})
}

func destinationLocation(d apiclient.StorageDestination) string {
	if d.Kind == "agent" {
		return "node " + d.NodeID + ":" + d.Directory
	}
	return d.Bucket
}

func quotaLabel(d apiclient.StorageDestination) string {
	if d.QuotaBytes == 0 {
		return "-"
	}
	label := humanBytes(d.QuotaBytes)
	if d.OverQuota {
		label += " (over)"
	}
	return label
}
