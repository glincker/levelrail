package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

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
	case "use":
		return runProfileUse(prog, args[1:], stdout, stderr)
	case "remove":
		return runProfileRemove(prog, args[1:], stdout, stderr)
	case "rename":
		return runProfileRename(prog, args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown profile subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, profileUsage(prog))
		return exitUsage
	}
}

func profileUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s profile list [flags]            list configured credentials profiles
  %[1]s profile use <name> [--json]            make a profile the default
  %[1]s profile remove <name> [--yes] [--json] delete a profile and its saved token
  %[1]s profile rename <old> <new> [--json]    rename a profile

A profile is a named section of the credentials file at
~/.config/%[1]s/credentials, each with its own %[2]s/%[3]s, written by
"%[1]s auth login --profile NAME". Select one for any command with
--profile NAME or the %[4]s env var; omitting both reads "%[5]s".

Run "%[1]s profile <subcommand> -h" for a subcommand's own flags.
`, prog, apiURLEnvHelp, apiTokenEnvHelp, envProfile, defaultProfile)
}

// Prints profile names and API URLs only, never tokens.
func runProfileList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" profile list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var jsonOut, verify bool
	fs.BoolVar(&verify, "verify", false, "check every saved token against its server and show identity and scope (one call per profile)")
	fs.BoolVar(&jsonOut, "json", false, "print profiles as a JSON array to stdout and nothing else")
	outputFlagP, queryFlagP := bindOutputQueryFlags(fs)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, profileListUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
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

	rows := buildProfileRows(prog, profiles, verify, lookupEnv)
	if err := renderResult(stdout, format, *queryFlagP, rows, func() { printProfileRows(stdout, rows) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func profileListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s profile list [flags]

Lists every profile configured in ~/.config/%[1]s/credentials: its name,
API URL, whether it is the active one, whether it holds a token, the token's
expiry and whether this machine is a trusted device, never the token value.
--verify also checks each token live and shows the identity and scope. A
profile with no usable token lists the exact command a person must run.

Flags:
  --verify                check each saved token against its server
  --json                  print profiles as a JSON array to stdout, nothing else
  --output string         output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string          JMESPath expression to filter the result before printing
  -h, --help              show this help
`, prog)
}

func profileMutationFlags(prog, label string, stderr io.Writer, args []string, want int) (jsonOut, yes *bool, of outputFlags, rest []string, code int, ok bool) {
	fs := flag.NewFlagSet(prog+" profile "+label, flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut = fs.Bool("json", false, "print the result as JSON to stdout and nothing else")
	yes = fs.Bool("yes", false, "skip the confirmation prompt")
	outputFlag, queryFlag := bindOutputQueryFlags(fs)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, profileUsage(prog)) }
	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, nil, outputFlags{}, nil, exitOK, false
		}
		return nil, nil, outputFlags{}, nil, exitUsage, false
	}
	format, err := resolveOutputFormat(*jsonOut, *outputFlag)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, err)
		return nil, nil, outputFlags{}, nil, exitValidation, false
	}
	if fs.NArg() != want {
		_, _ = fmt.Fprint(stderr, profileUsage(prog))
		return nil, nil, outputFlags{}, nil, exitUsage, false
	}
	return jsonOut, yes, outputFlags{format, *queryFlag}, fs.Args(), 0, true
}

func runProfileUse(prog string, args []string, stdout, stderr io.Writer) int {
	jsonOut, _, of, rest, code, ok := profileMutationFlags(prog, "use", stderr, args, 1)
	if !ok {
		return code
	}
	return runConfigUseContextNamed(prog, rest[0], *jsonOut, of, stdout, stderr)
}

func runProfileRemove(prog string, args []string, stdout, stderr io.Writer) int {
	jsonOut, yes, of, rest, code, ok := profileMutationFlags(prog, "remove", stderr, args, 1)
	if !ok {
		return code
	}
	return removeProfileCommand(prog, rest[0], *yes, *jsonOut, of, stdout, stderr)
}

func removeProfileCommand(prog, name string, yes, jsonOut bool, of outputFlags, stdout, stderr io.Writer) int {
	if err := confirmAction(stdinSource, stderr, fmt.Sprintf("Remove profile %s and its saved token", name), yes, "--yes"); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if err := removeProfile(prog, name); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if err := renderResult(stdout, of.Format, of.Query, map[string]any{"removed": name}, func() {
		_, _ = fmt.Fprintf(stdout, "Removed profile %q.\n", name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runProfileRename(prog string, args []string, stdout, stderr io.Writer) int {
	jsonOut, _, of, rest, code, ok := profileMutationFlags(prog, "rename", stderr, args, 2)
	if !ok {
		return code
	}
	return renameProfileCommand(prog, rest[0], rest[1], *jsonOut, of, stdout, stderr)
}

func renameProfileCommand(prog, from, to string, jsonOut bool, of outputFlags, stdout, stderr io.Writer) int {
	if err := renameProfile(prog, from, to); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if err := renderResult(stdout, of.Format, of.Query, map[string]string{"from": from, "to": to}, func() {
		_, _ = fmt.Fprintf(stdout, "Renamed profile %q to %q.\n", from, to)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// savedProfileNames lists every saved profile name, for verbs that fan out over them.
func savedProfileNames(prog string) ([]string, error) {
	profiles, err := listProfiles(prog)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(profiles))
	for _, p := range profiles {
		names = append(names, p.Name)
	}
	return names, nil
}
