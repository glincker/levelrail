package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func obsPresetsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s observability presets [flags]                       list the provider presets and their form fields
  %[1]s observability add <preset> --name N --field K=V [--signal logs|metrics] [flags]
                                                          create an export from a preset
  %[1]s observability test <name> [flags]                   send one log line and metric, report the result
  %[1]s observability list [flags]                          list exports and their delivery state
  %[1]s observability rm <name> [flags]                     delete an export and its stored credentials

Run "%[1]s observability presets" to see the fields each preset takes. A field value written as
env:NAME is read from that environment variable, so a token stays out of shell history.
Credentials are stored encrypted and are never shown again.
`, prog)
}

// runObservability handles "observability ...", a provider-first view over
// the data export destinations.
func runObservability(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, obsPresetsUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, obsPresetsUsage(prog))
		return exitOK
	case "presets":
		return runObservabilityPresets(prog, rest, stdout, stderr, lookupEnv)
	case "add":
		return runObservabilityAdd(prog, rest, stdout, stderr, lookupEnv)
	case "test":
		return runObservabilityTest(prog, rest, stdout, stderr, lookupEnv)
	case "list":
		return runObservabilityList(prog, rest, stdout, stderr, lookupEnv)
	case "rm":
		return runObservabilityRm(prog, rest, stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown observability subcommand %q\n\n%s", prog, args[0], obsPresetsUsage(prog))
	return exitUsage
}

func runObservabilityPresets(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "observability presets", obsPresetsUsage(prog)+commonFlagsHelp, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	cat, err := c.client.ListObservabilityPresets(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("list observability presets: %w", err))
	}
	return c.render(cat, func() {
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "PRESET\tNAME\tFIELDS\tSIGNALS")
		for _, p := range cat.Presets {
			var fields, sigs []string
			for _, f := range p.Fields {
				label := f.Key
				if f.Required {
					label += "*"
				}
				if f.Secret {
					label += " (secret)"
				}
				fields = append(fields, label)
			}
			for _, s := range p.Signals {
				sigs = append(sigs, s.Signal)
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.ID, p.Name, strings.Join(fields, ", "), strings.Join(sigs, ","))
		}
		_ = tw.Flush()
	})
}

func resolveObservabilityFields(raw fieldFlags, lookupEnv func(string) (string, bool)) (map[string]string, error) {
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if name, ok := strings.CutPrefix(v, "env:"); ok {
			val, found := lookupEnv(name)
			if !found || val == "" {
				return nil, fmt.Errorf("field %q: environment variable %s is not set", k, name)
			}
			v = val
		}
		out[k] = v
	}
	return out, nil
}

func runObservabilityAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	// The preset id is positional and may come first, before the flags.
	var preset string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		preset, args = args[0], args[1:]
	}
	var name, signals string
	fields := fieldFlags{}
	c, code, ok := parseCLICall(prog, "observability add", obsPresetsUsage(prog)+commonFlagsHelp, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&name, "name", "", "export name (lowercase letters, digits, dashes)")
		fs.StringVar(&signals, "signal", "", "comma separated signals, default per preset")
		fs.Var(fields, "field", "preset field K=V or K=env:NAME (repeatable)")
	})
	if !ok {
		return code
	}
	if preset == "" {
		preset, ok = requireOneArg(c.fs, stderr, prog, "observability add", "preset id")
		if !ok {
			return exitUsage
		}
	}
	if name == "" {
		name = preset
	}
	resolved, err := resolveObservabilityFields(fields, lookupEnv)
	if err != nil {
		return c.invalid("%v", err)
	}
	req := apiclient.CreateObservabilityExportRequest{Preset: preset, Name: name, Fields: resolved}
	if signals != "" {
		req.Signals = strings.Split(signals, ",")
	}
	created, err := c.client.CreateObservabilityExport(context.Background(), req)
	if err != nil {
		return c.fail(fmt.Errorf("add observability export %q: %w", name, err))
	}
	return c.render(created, func() {
		_, _ = fmt.Fprintf(stdout, "export %q created from %s\n", created.ID, created.PresetName)
		for _, s := range created.Signals {
			_, _ = fmt.Fprintf(stdout, "  %s -> %s\n", s.Signal, s.Endpoint)
		}
		_, _ = fmt.Fprintf(stdout, "run: %s observability test %s\n", prog, created.ID)
	})
}

func runObservabilityTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := observabilityIDCall(prog, "observability test", args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	res, err := c.client.TestObservabilityExport(context.Background(), id)
	if err != nil {
		return c.fail(fmt.Errorf("test observability export %q: %w", id, err))
	}
	status := c.render(res, func() {
		for _, r := range res.Results {
			if r.OK {
				_, _ = fmt.Fprintf(stdout, "%s: accepted\n", r.Signal)
				continue
			}
			_, _ = fmt.Fprintf(stdout, "%s: failed: %s\n", r.Signal, r.Error)
			if r.Hint != "" {
				_, _ = fmt.Fprintf(stdout, "  fix: %s\n", r.Hint)
			}
		}
	})
	if status == exitOK && !res.OK {
		return exitCheckFailed
	}
	return status
}

func runObservabilityList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "observability list", obsPresetsUsage(prog)+commonFlagsHelp, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	list, err := c.client.ListObservabilityExports(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("list observability exports: %w", err))
	}
	return c.render(list, func() {
		if len(list) == 0 {
			_, _ = fmt.Fprintln(stdout, "no observability exports")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "NAME\tPRESET\tSIGNAL\tENDPOINT\tSENT\tLAST ERROR")
		for _, e := range list {
			for _, s := range e.Signals {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n", e.ID, e.Preset, s.Signal, s.Endpoint, s.SentTotal, s.LastError)
			}
		}
		_ = tw.Flush()
	})
}

func runObservabilityRm(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := observabilityIDCall(prog, "observability rm", args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	if err := c.client.DeleteObservabilityExport(context.Background(), id); err != nil {
		return c.fail(fmt.Errorf("remove observability export %q: %w", id, err))
	}
	return c.render(map[string]string{"deleted": id}, func() {
		_, _ = fmt.Fprintf(stdout, "export %q removed\n", id)
	})
}

func observabilityIDCall(prog, label string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) (cliCall, string, int, bool) {
	usage := fmt.Sprintf("Usage:\n  %s %s <name> [flags]\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return cliCall{}, "", code, false
	}
	id, ok := requireOneArg(c.fs, stderr, prog, label, "export name")
	if !ok {
		return cliCall{}, "", exitUsage, false
	}
	return c, id, exitOK, true
}
