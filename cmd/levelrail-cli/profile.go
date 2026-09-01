package main

import (
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

// runProfile dispatches "profile <verb> [flags]" to one of list, the CLI
// counterpart of the named-profile sections config.go's
// resolveToken/resolveAPIURL read from the credentials file (see
// internal/apiclient's own ReadCredentialsFile/WriteCredentialsFile doc
// comments).
func runProfile(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, profileUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, profileUsage(prog))
		return exitOK
	case "list":
		return runProfileList(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown profile subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, profileUsage(prog))
		return exitUsage
	}
}

func profileUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s profile list [flags]   list configured credentials profiles

A profile is a named section of the credentials file at
~/.config/%[1]s/credentials, each with its own %[2]s/%[3]s, written by
"%[1]s auth login --profile NAME". Select one for any command with
--profile NAME or the %[4]s env var; omitting both reads "%[5]s".

Run "%[1]s profile <subcommand> -h" for a subcommand's own flags.
`, prog, envAPIURL, envAPIToken, envProfile, defaultProfile)
}

// runProfileList implements "profile list": every profile section in
// prog's credentials file, its name and API URL, never its token (this
// CLI's standing "never print a secret" rule, the same reason "auth
// login" only ever shows a freshly minted token once and nowhere else).
func runProfileList(prog string, args []string, stdout, stderr io.Writer, _ func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" profile list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var jsonOut bool
	fs.BoolVar(&jsonOut, "json", false, "print profiles as a JSON array to stdout and nothing else")
	outputFlagP, queryFlagP := bindOutputQueryFlags(fs)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, profileListUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	format, ferr := resolveOutputFormat(jsonOut, *outputFlagP)
	if ferr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, ferr)
		return exitValidation
	}

	profiles, err := listProfiles(prog)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list profiles: %w", err))
	}

	if err := renderResult(stdout, format, *queryFlagP, profiles, func() { printProfilesTable(stdout, profiles) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printProfilesTable(out io.Writer, profiles []profileSummary) {
	if len(profiles) == 0 {
		_, _ = fmt.Fprintln(out, "no profiles configured")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tAPI URL")
	for _, p := range profiles {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", p.Name, p.APIURL)
	}
	_ = tw.Flush()
}

func profileListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s profile list [flags]

Lists every profile configured in ~/.config/%[1]s/credentials: its name
and API URL, never its token value.

Flags:
  --json                  print profiles as a JSON array to stdout, nothing else
  --output string         output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string          JMESPath expression to filter the result before printing
  -h, --help              show this help
`, prog)
}
