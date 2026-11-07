package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func bootstrapScriptUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s bootstrap-script list [flags]                                  list saved node bootstrap scripts
  %[1]s bootstrap-script show <id> [flags]                             print one script, body included
  %[1]s bootstrap-script create --name NAME --file PATH [flags]        save a script (use --file - for stdin)
  %[1]s bootstrap-script delete <id> [flags]                           remove a saved script

A saved script runs once on a new cloud node's first boot, after the built-in
bootstrap, when a node is provisioned with its bootstrap_script_id. The control
plane stores it as text and never runs it. Do not put secrets in a script.
`, prog)
}

func runBootstrapScript(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, bootstrapScriptUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, bootstrapScriptUsage(prog))
		return exitOK
	case "list":
		return runBootstrapScriptList(prog, args[1:], stdout, stderr, lookupEnv)
	case "show":
		return runBootstrapScriptShow(prog, args[1:], stdout, stderr, lookupEnv)
	case "create":
		return runBootstrapScriptCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runBootstrapScriptDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown bootstrap-script subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, bootstrapScriptUsage(prog))
		return exitUsage
	}
}

func runBootstrapScriptList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "bootstrap-script list", "print the scripts as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, bootstrapScriptUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.ListBootstrapScripts(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list bootstrap scripts: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, list, func() {
		if len(list) == 0 {
			_, _ = fmt.Fprintln(stdout, "no bootstrap scripts")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tNAME\tBYTES\tCREATED BY\tUPDATED")
		for _, s := range list {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n", s.ID, s.Name, s.SizeBytes, s.CreatedBy, s.UpdatedAt.Format("2006-01-02 15:04"))
		}
		_ = tw.Flush()
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runBootstrapScriptShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "bootstrap-script show", "print the script as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, bootstrapScriptUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "bootstrap-script show", "bootstrap script id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	s, err := client.GetBootstrapScript(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("show bootstrap script %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, s, func() {
		_, _ = fmt.Fprintf(stdout, "# %s (%s)\n%s", s.Name, s.ID, s.Body)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runBootstrapScriptCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "bootstrap-script create", "print the created script as JSON to stdout and nothing else", stderr)
	var name, file string
	fs.StringVar(&name, "name", "", "unique name for the script (required)")
	fs.StringVar(&file, "file", "", "path of the script file, or - for stdin (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, bootstrapScriptUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if name == "" || file == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name and --file are required"))
	}
	var body []byte
	var err error
	if file == "-" {
		body, err = io.ReadAll(os.Stdin)
	} else {
		body, err = os.ReadFile(file) //nolint:gosec // operator-chosen script path
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("read script: %w", err))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	created, err := client.CreateBootstrapScript(context.Background(), apiclient.BootstrapScriptRequest{Name: name, Body: string(body)})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create bootstrap script %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		_, _ = fmt.Fprintf(stdout, "saved bootstrap script %q as %s\n", created.Name, created.ID)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runBootstrapScriptDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "bootstrap-script delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, bootstrapScriptUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "bootstrap-script delete", "bootstrap script id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DeleteBootstrapScript(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete bootstrap script %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "bootstrap script %q deleted\n", id)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
