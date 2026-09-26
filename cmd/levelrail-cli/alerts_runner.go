package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
)

// apiCmd describes one leaf command of the alerts and status-page trees:
// its flags, how many positional arguments it takes and what it does.
// runAPICmd owns the shared parse, client and render plumbing.
type apiCmd struct {
	label string
	usage func(prog string) string
	args  int
	setup func(fs *flag.FlagSet)
	// run returns the value for --json/--output plus a table printer for the default view.
	run func(ctx context.Context, c *Client, pos []string) (data any, table func(io.Writer), err error)
}

func runAPICmd(prog string, cmd apiCmd, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, cmd.label, "print the result as JSON to stdout and nothing else", stderr)
	if cmd.setup != nil {
		cmd.setup(fs)
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, cmd.usage(prog)) }

	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return code
	}
	pos, ok := requireArgs(fs, stderr, prog, cmd.label, fmt.Sprintf("%d argument(s)", cmd.args), cmd.args)
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	data, table, err := cmd.run(context.Background(), client, pos)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", cmd.label, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, data, func() { table(stdout) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// stringList is a repeatable string flag.
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// labelMap parses repeated key=value flag values.
func labelMap(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, newValidationError("--label %q must be key=value", p)
		}
		out[k] = v
	}
	return out, nil
}

// dispatchSub routes "<group> <verb>" to a leaf command or prints usage.
func dispatchSub(prog, group string, usage func(string) string, subs map[string]apiCmd, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, usage(prog))
		return exitOK
	}
	cmd, ok := subs[args[0]]
	if !ok {
		_, _ = fmt.Fprintf(stderr, "%s: unknown %s subcommand %q\n\n", prog, group, args[0])
		_, _ = fmt.Fprint(stderr, usage(prog))
		return exitUsage
	}
	return runAPICmd(prog, cmd, args[1:], stdout, stderr, lookupEnv)
}
