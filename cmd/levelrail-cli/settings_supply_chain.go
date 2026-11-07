package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func settingsSupplyChainUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings supply-chain get [flags]
  %[1]s settings supply-chain set [--scan on|off] [--gate off|warn|block_on_critical|block_on_high] [flags]

The instance-wide image scanning policy. Apps without their own policy
(apps scan enable|gate) inherit it, so "scan every app, block critical"
is set once. Saved values apply at once.
%[2]s`, prog, commonFlagsHelp)
}

func runSettingsSupplyChain(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsSupplyChainUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsSupplyChainUsage(prog))
		return exitOK
	case "get":
		return runSettingsSupplyChainGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runSettingsSupplyChainSet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings supply-chain subcommand %q\n\n%s", prog, args[0], settingsSupplyChainUsage(prog))
		return exitUsage
	}
}

func printSupplyChainDefault(w io.Writer, d apiclient.SupplyChainDefault) {
	_, _ = fmt.Fprintf(w, "scan new apps: %t\ngate:          %s\nserver:        %t\n", d.ScanEnabled, d.ScanGate, d.ServerEnabled)
}

func runSettingsSupplyChainGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "settings supply-chain get", settingsSupplyChainUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	res, err := c.client.GetSupplyChainDefault(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("get supply chain default: %w", err))
	}
	return c.render(res, func() { printSupplyChainDefault(stdout, res) })
}

func runSettingsSupplyChainSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var scan, gate string
	c, code, ok := parseCLICall(prog, "settings supply-chain set", settingsSupplyChainUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&scan, "scan", "", "scan apps without their own policy: on or off")
		fs.StringVar(&gate, "gate", "", "what a scan may do to a release: off, warn, block_on_critical or block_on_high")
	})
	if !ok {
		return code
	}
	var req apiclient.SupplyChainSettingsRequest
	switch scan {
	case "":
	case "on", "off":
		v := scan == "on"
		req.ScanEnabled = &v
	default:
		return c.invalid("--scan must be on or off")
	}
	if gate != "" {
		req.ScanGate = &gate
	}
	if req.ScanEnabled == nil && req.ScanGate == nil {
		return c.invalid("expected --scan and/or --gate")
	}
	res, err := c.client.SetSupplyChainDefault(context.Background(), req)
	if err != nil {
		return c.fail(fmt.Errorf("set supply chain default: %w", err))
	}
	return c.render(res, func() { printSupplyChainDefault(stdout, res) })
}
