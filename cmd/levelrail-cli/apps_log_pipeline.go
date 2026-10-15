package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func appsLogPipelineUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps log-pipeline get <app> [flags]
  %[1]s apps log-pipeline set <app> --file rules.json [flags]
  %[1]s apps log-pipeline clear <app> [flags]
  %[1]s apps log-pipeline preview <app> --input lines.txt [--file rules.json] [flags]

Rules run in order on an app's log lines before they are stored. The file holds
{"rules":[{"kind":"redact|parse|drop|multiline","pattern":"...","replace":"..."}]}.
preview uses the app's saved rules when --file is omitted; --input - reads stdin.
`, prog)
}

func readLogPipelineFile(path string) ([]apiclient.LogPipelineRule, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // operator-supplied rules file
	if err != nil {
		return nil, fmt.Errorf("read rules file: %w", err)
	}
	var p apiclient.LogPipeline
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("parse rules file: %w", err)
	}
	return p.Rules, nil
}

func runAppsLogPipeline(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprint(stdout, appsLogPipelineUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	verb := args[0]
	switch verb {
	case "get", "set", "clear", "preview":
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps log-pipeline subcommand %q\n\n%s", prog, verb, appsLogPipelineUsage(prog))
		return exitUsage
	}
	var file, input string
	label := "apps log-pipeline " + verb
	usage := fmt.Sprintf("Usage:\n  %s %s <app> [flags]\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args[1:], stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		if verb == "set" || verb == "preview" {
			fs.StringVar(&file, "file", "", "JSON file with the rules")
		}
		if verb == "preview" {
			fs.StringVar(&input, "input", "", "file of sample log lines, or - for stdin")
		}
	})
	if !ok {
		return code
	}
	app, ok := requireOneArg(c.fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	ctx := context.Background()
	switch verb {
	case "set":
		if file == "" {
			_, _ = fmt.Fprintf(stderr, "%s: --file is required\n", prog)
			return exitUsage
		}
		rules, err := readLogPipelineFile(file)
		if err != nil {
			return c.fail(err)
		}
		res, err := c.client.SetLogPipeline(ctx, app, rules)
		if err != nil {
			return c.fail(fmt.Errorf("set log pipeline for %q: %w", app, err))
		}
		return c.render(res, func() {
			_, _ = fmt.Fprintf(stdout, "log pipeline for %s saved (%d rules)\n", app, len(res.Rules))
		})
	case "clear":
		if err := c.client.DeleteLogPipeline(ctx, app); err != nil {
			return c.fail(fmt.Errorf("clear log pipeline for %q: %w", app, err))
		}
		return c.render(map[string]bool{"cleared": true}, func() {
			_, _ = fmt.Fprintf(stdout, "log pipeline for %s cleared\n", app)
		})
	case "preview":
		return runLogPipelinePreview(ctx, c, app, file, input, stdout, stderr)
	default:
		res, err := c.client.GetLogPipeline(ctx, app)
		if err != nil {
			return c.fail(fmt.Errorf("get log pipeline for %q: %w", app, err))
		}
		return c.render(res, func() {
			if len(res.Rules) == 0 {
				_, _ = fmt.Fprintf(stdout, "no log pipeline for %s\n", app)
				return
			}
			for i, r := range res.Rules {
				_, _ = fmt.Fprintf(stdout, "%d. %s %s\n", i+1, r.Kind, r.Pattern)
			}
		})
	}
}

func runLogPipelinePreview(ctx context.Context, c cliCall, app, file, input string, stdout, stderr io.Writer) int {
	if input == "" {
		_, _ = fmt.Fprintln(stderr, "--input is required")
		return exitUsage
	}
	var raw []byte
	var err error
	if input == "-" {
		raw, err = io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	} else {
		raw, err = os.ReadFile(input) //nolint:gosec // operator-supplied sample file
	}
	if err != nil {
		return c.fail(fmt.Errorf("read input lines: %w", err))
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\r\n"), "\n")
	var rules []apiclient.LogPipelineRule
	if file != "" {
		if rules, err = readLogPipelineFile(file); err != nil {
			return c.fail(err)
		}
	} else {
		saved, gerr := c.client.GetLogPipeline(ctx, app)
		if gerr != nil {
			return c.fail(fmt.Errorf("get log pipeline for %q: %w", app, gerr))
		}
		rules = saved.Rules
	}
	res, err := c.client.PreviewLogPipeline(ctx, app, rules, lines)
	if err != nil {
		return c.fail(fmt.Errorf("preview log pipeline for %q: %w", app, err))
	}
	return c.render(res, func() {
		for i, l := range res.Lines {
			switch {
			case l.Dropped:
				_, _ = fmt.Fprintf(stdout, "%d. (dropped)\n", i+1)
			default:
				_, _ = fmt.Fprintf(stdout, "%d. %s\n", i+1, l.Message)
				for k, v := range l.Fields {
					_, _ = fmt.Fprintf(stdout, "     %s=%s\n", k, v)
				}
			}
		}
	})
}
