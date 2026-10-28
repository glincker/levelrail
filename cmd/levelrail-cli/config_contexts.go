package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const currentContextFile = "current-context"

type contextRow struct {
	Current bool   `json:"current"`
	Name    string `json:"name"`
	APIURL  string `json:"apiUrl"`
	Team    string `json:"team,omitempty"`
	Project string `json:"project,omitempty"`
}

func readCurrentContext(prog string) string {
	dir, err := apiclient.ConfigDir(prog)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, currentContextFile)) //nolint:gosec // fixed file name under the config dir
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func writeCurrentContext(prog, name string) error {
	dir, err := apiclient.ConfigDir(prog)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, currentContextFile), []byte(name+"\n"), 0o600)
}

// A saved current context only fills in the profile when neither --profile nor the env pick one.
func currentContextLookup(prog string, next func(string) (string, bool)) func(string) (string, bool) {
	saved := readCurrentContext(prog)
	if saved == "" {
		return next
	}
	return func(key string) (string, bool) {
		if key == envProfile {
			if v, ok := next(key); ok && v != "" {
				return v, true
			}
			return saved, true
		}
		return next(key)
	}
}

func runConfig(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, configUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, configUsage(prog))
		return exitOK
	case "get-contexts":
		return runConfigGetContexts(prog, args[1:], stdout, stderr, lookupEnv)
	case "current-context":
		return runConfigCurrentContext(prog, args[1:], stdout, stderr, lookupEnv)
	case "use-context":
		return runConfigUseContext(prog, args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown config subcommand %q\n\n%s", prog, args[0], configUsage(prog))
		return exitUsage
	}
}

func contextRows(prog, current string) ([]contextRow, error) {
	profiles, err := listProfiles(prog)
	if err != nil {
		return nil, err
	}
	rows := make([]contextRow, 0, len(profiles))
	for _, p := range profiles {
		sc, _ := apiclient.ReadContext(prog, p.Name)
		rows = append(rows, contextRow{Current: p.Name == current, Name: p.Name, APIURL: p.APIURL, Team: sc.Team, Project: sc.Project})
	}
	return rows, nil
}

func configFlagSet(prog, label string, stderr io.Writer, usage string) (*flag.FlagSet, *bool, *string, *string) {
	fs := flag.NewFlagSet(prog+" config "+label, flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "print the result as JSON to stdout and nothing else")
	outputFlag, queryFlag := bindOutputQueryFlags(fs)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, usage) }
	return fs, jsonOut, outputFlag, queryFlag
}

func runConfigGetContexts(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, jsonOut, outputFlag, queryFlag := configFlagSet(prog, "get-contexts", stderr, configUsage(prog))
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	format, err := resolveOutputFormat(*jsonOut, *outputFlag)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, err)
		return exitValidation
	}
	rows, err := contextRows(prog, resolveProfile("", lookupEnv))
	if err != nil {
		return reportError(stdout, stderr, *jsonOut, fmt.Errorf("list contexts: %w", err))
	}
	if err := renderResult(stdout, format, *queryFlag, rows, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "CURRENT\tNAME\tAPI URL\tTEAM\tPROJECT")
		for _, r := range rows {
			mark := ""
			if r.Current {
				mark = "*"
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", mark, r.Name, r.APIURL, r.Team, r.Project)
		}
		_ = tw.Flush()
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runConfigCurrentContext(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, jsonOut, outputFlag, queryFlag := configFlagSet(prog, "current-context", stderr, configUsage(prog))
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	format, err := resolveOutputFormat(*jsonOut, *outputFlag)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, err)
		return exitValidation
	}
	name := resolveProfile("", lookupEnv)
	if format == outputTable && *queryFlag == "" {
		_, _ = fmt.Fprintln(stdout, name)
		return exitOK
	}
	if err := renderResult(stdout, format, *queryFlag, map[string]string{"name": name}, func() {}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runConfigUseContext(prog string, args []string, stdout, stderr io.Writer) int {
	fs, jsonOut, outputFlag, queryFlag := configFlagSet(prog, "use-context", stderr, configUsage(prog))
	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	format, err := resolveOutputFormat(*jsonOut, *outputFlag)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, err)
		return exitValidation
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprint(stderr, configUsage(prog))
		return exitUsage
	}
	name := fs.Arg(0)
	profiles, err := listProfiles(prog)
	if err != nil {
		return reportError(stdout, stderr, *jsonOut, fmt.Errorf("list contexts: %w", err))
	}
	found := false
	for _, p := range profiles {
		found = found || p.Name == name
	}
	if !found {
		return reportError(stdout, stderr, *jsonOut, newValidationError("no context %q, run \"%s config get-contexts\" to list them", name, prog))
	}
	if err := writeCurrentContext(prog, name); err != nil {
		return reportError(stdout, stderr, *jsonOut, err)
	}
	if err := renderResult(stdout, format, *queryFlag, map[string]string{"current": name}, func() {
		_, _ = fmt.Fprintf(stdout, "Switched to context %q.\n", name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func configUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s config get-contexts [flags]        list saved profiles, marking the current one
  %[1]s config current-context [flags]     print the active profile name
  %[1]s config use-context <name> [flags]  make a profile the default

A context is a credentials profile. The --profile flag and the APP_PROFILE
environment variable still win over the saved current context.
`, prog)
}
