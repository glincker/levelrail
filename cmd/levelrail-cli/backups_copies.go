package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runBackupsDestinations implements "backups destinations copy [show|set]".
func runBackupsDestinations(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	usage := fmt.Sprintf(`Usage:
  %[1]s backups destinations copy show [flags]
  %[1]s backups destinations copy set <destination> [flags]
  %[1]s backups destinations copy set off [flags]

Shows or changes the second destination every new backup is copied to once it
lands in its primary destination. Only backups finished after copying is turned
on are copied. Changing the setting needs a write-sensitive token.
`, prog)
	if len(args) < 2 || (args[1] != "show" && args[1] != "set") {
		_, _ = fmt.Fprint(stderr, usage)
		return exitUsage
	}
	switch args[0] {
	case "copy":
	default:
		_, _ = fmt.Fprint(stderr, usage)
		return exitUsage
	}
	verb := args[1]
	want := 0
	if verb == "set" {
		want = 1
	}
	c := newProtectionCmd(prog, "backups destinations copy "+verb, usage, stdout, stderr, lookupEnv)
	rest, exit, ok := c.parse(args[2:], want)
	if !ok {
		return exit
	}
	var res apiclient.BackupCopySettings
	var err error
	if verb == "show" {
		res, err = c.client.GetBackupCopySettings(context.Background())
	} else if rest[0] == "off" {
		res, err = c.client.SetBackupCopySettings(context.Background(), false, "")
	} else {
		res, err = c.client.SetBackupCopySettings(context.Background(), true, rest[0])
	}
	if err != nil {
		return c.fail(fmt.Errorf("backup copy settings: %w", err))
	}
	return c.render(res, func() {
		if !res.Enabled {
			_, _ = fmt.Fprintln(stdout, "backup copies: off")
			return
		}
		_, _ = fmt.Fprintf(stdout, "backup copies: on, second destination %s (%s)\n", res.TargetName, res.TargetID)
	})
}

// runBackupsCopies implements "backups copies list".
func runBackupsCopies(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	usage := fmt.Sprintf(`Usage:
  %[1]s backups copies list [--limit N] [flags]

Lists each backup's copy to the second destination with its status and lag.
`, prog)
	if len(args) == 0 || args[0] != "list" {
		_, _ = fmt.Fprint(stderr, usage)
		return exitUsage
	}
	c := newProtectionCmd(prog, "backups copies list", usage, stdout, stderr, lookupEnv)
	limit := c.fs.Int("limit", 0, "maximum rows to return (default: server default)")
	if _, exit, ok := c.parse(args[1:], 0); !ok {
		return exit
	}
	copies, err := c.client.ListBackupCopies(context.Background(), *limit)
	if err != nil {
		return c.fail(fmt.Errorf("list backup copies: %w", err))
	}
	return c.render(copies, func() { printBackupCopies(stdout, copies) })
}

func printBackupCopies(out io.Writer, copies []apiclient.BackupCopy) {
	if len(copies) == 0 {
		_, _ = fmt.Fprintln(out, "no backup copies")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "BACKUP\tRESOURCE\tSTATUS\tLAG\tATTEMPTS\tERROR")
	for _, cp := range copies {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", cp.BackupID, cp.Resource, cp.Status,
			(time.Duration(cp.LagSeconds) * time.Second).String(), strconv.Itoa(cp.Attempts), cp.Error)
	}
	_ = tw.Flush()
}
