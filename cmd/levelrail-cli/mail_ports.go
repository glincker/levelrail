package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const mailPortsUsageLines = `  %[1]s mail ports [--app name]                  list the four mail ports, their bind and firewall state
  %[1]s mail ports open --confirm [--app name]    make 25, 465, 587, 993 public and allow them in the firewall (asks your password)
`

// runMailPorts handles "mail ports" and "mail ports open".
func runMailPorts(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	open := len(args) > 0 && args[0] == "open"
	if open {
		args = args[1:]
	}
	var app string
	var confirm bool
	name := "mail ports"
	if open {
		name = "mail ports open"
	}
	c, code, ok := parseDBAccess(prog, name, name+" [--app name]", args, 0, "", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&app, "app", "", "mail app when several are installed")
		if open {
			fs.BoolVar(&confirm, "confirm", false, "acknowledge that the ports go on the public internet")
		}
	})
	if !ok {
		return code
	}
	if open {
		if !confirm {
			return c.fail(fmt.Errorf("mail ports open: pass --confirm after reading the warning: the ports go on the public internet"))
		}
		res, err := withStepUp(stderr, func(p *apiclient.StepUpProof) (apiclient.MailPortsOpenResult, error) {
			return c.client.OpenMailPorts(context.Background(), app, confirm, p)
		})
		if err != nil {
			return c.fail(fmt.Errorf("mail ports open: %w", err))
		}
		return c.result(res, func() {
			_, _ = fmt.Fprintf(stdout, "app: %s\nresult: %s\nchanged: %v\n%s\n\nWarning: %s\n", res.App, res.Result, res.Changed, res.Message, res.Warning)
		})
	}
	res, err := c.client.MailPorts(context.Background(), app)
	if err != nil {
		return c.fail(fmt.Errorf("mail ports: %w", err))
	}
	return c.result(res, func() { printMailPorts(stdout, res) })
}

func printMailPorts(stdout io.Writer, res apiclient.MailPorts) {
	if !res.Installed {
		_, _ = fmt.Fprintln(stdout, res.NextStep)
		return
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PORT\tBIND\tFIREWALL\tPURPOSE")
	for _, p := range res.Ports {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", p.Port, p.Bind, p.Firewall, p.Purpose)
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(stdout, "\nweb listener: %s\nport 25: %s\nwarning: %s\nnote: %s\n", res.WebListener, res.Port25Note, res.Warning, res.BindScopeNote)
}
