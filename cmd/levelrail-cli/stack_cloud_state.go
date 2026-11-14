package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

func stackCloudStateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s stack cloud-state <command> [flags]

Commands:
  list <name>                              unresolved quarantined cloud state of a stack
  recover <name> <id> --confirm <name>     clear one quarantine row (root only)

Run '%[1]s stack cloud-state <command> --help' for the flags of a command.
`, prog)
}

func runStackCloudState(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprint(stderr, stackCloudStateUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	switch args[0] {
	case "list":
		return stackCloudStateList(prog, args[1:], stdout, stderr, lookupEnv)
	case "recover":
		return stackCloudStateRecover(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s stack cloud-state: unknown command %q\n\n%s", prog, args[0], stackCloudStateUsage(prog))
	return exitUsage
}

func stackCloudStateList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "stack cloud-state list", stackCmdUsage(prog, "cloud-state list", "<name>", `
Lists state the stack could not store after a cloud apply. While a row is
unresolved every further cloud plan and apply of the stack is refused.
`), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "cloud-state list", "<name>", 1)
	if !ok {
		return code
	}
	list, raw, err := c.client.ListStackCloudQuarantine(context.Background(), pos[0])
	if err != nil {
		return iacFail(c, err)
	}
	return c.render(raw, func() {
		if len(list.Quarantine) == 0 {
			_, _ = fmt.Fprintf(c.stdout, "stack %s has no quarantined cloud state\n", pos[0])
			return
		}
		tw := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tACCOUNT\tFENCE\tCREATED\tREASON")
		for _, q := range list.Quarantine {
			_, _ = fmt.Fprintf(tw, "%d\t%s\t%d\t%s\t%s\n", q.ID, q.Account, q.Fence, q.CreatedAt, q.Reason)
		}
		_ = tw.Flush()
	})
}

func stackCloudStateRecover(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var confirm *string
	c, code, ok := parseCLICall(prog, "stack cloud-state recover", stackCmdUsage(prog, "cloud-state recover", "<name> <id> --confirm <name>", `
Marks one quarantine row resolved. That only lifts the block on cloud runs: it
does not restore the state. Reconcile the real resources and the state by hand
first, or the next apply can recreate resources that already exist. Needs root.

Flags:
  --confirm NAME   the stack name, typed (required)
`), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		confirm = fs.String("confirm", "", "the stack name, typed")
	})
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "cloud-state recover", "<name> <id>", 2)
	if !ok {
		return code
	}
	if strings.TrimSpace(*confirm) != pos[0] {
		return c.invalid("--confirm must equal the stack name %q", pos[0])
	}
	raw, err := c.client.ResolveStackCloudQuarantine(context.Background(), pos[0], pos[1], *confirm)
	if err != nil {
		return iacFail(c, err)
	}
	return c.render(raw, func() {
		_, _ = fmt.Fprintf(c.stdout, "quarantine row %s of stack %s marked resolved; the stack can run again\n", pos[1], pos[0])
	})
}
