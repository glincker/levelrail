package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
)

func runDomainsExport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var format, out string
	label := "domains export"
	usage := fmt.Sprintf("Usage:\n  %s %s [flags]\n\nExport every domain and parked domain as CSV (names and state only, no settings or credentials).\n\nFlags:\n  --format string   export format; only csv is supported (default csv)\n  --out string      write the export to this file (default stdout)\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&format, "format", "csv", "export format (csv)")
		fs.StringVar(&out, "out", "", "write the export to this file")
	})
	if !ok {
		return code
	}
	if format != "csv" {
		_, _ = fmt.Fprintln(stderr, "--format must be csv")
		return exitUsage
	}
	data, err := c.client.ExportDomainsCSV(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("export domains: %w", err))
	}
	if out == "" {
		_, _ = stdout.Write(data)
		return exitOK
	}
	if err := os.WriteFile(out, data, 0o600); err != nil {
		return c.fail(fmt.Errorf("write %s: %w", out, err))
	}
	_, _ = fmt.Fprintf(stdout, "wrote %s\n", out)
	return exitOK
}
