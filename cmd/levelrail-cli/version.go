package main

import (
	"context"
	"fmt"
	"io"
	"runtime/debug"

	"github.com/GLINCKER/levelrail/internal/version"
)

const (
	unknownBuildInfo = "unknown"
	shortCommitLen   = 12
)

type cliVersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
}

type versionReport struct {
	CLI           cliVersionInfo   `json:"cli"`
	Server        *updatesResource `json:"server,omitempty"`
	ServerMatches *bool            `json:"server_matches,omitempty"`
}

// runVersion prints the CLI's own build info offline; only --server touches the network.
func runVersion(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "version", "print version info as JSON to stdout and nothing else", stderr)
	serverFlag := fs.Bool("server", false, "also query the control plane for its running version and latest release")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, versionUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	report := versionReport{CLI: localVersionInfo()}
	if *serverFlag {
		client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
		updates, err := client.GetUpdates(context.Background())
		if err != nil {
			serverURL := resolveAPIURL(apiURLFlag, lookupEnv, prog, resolveProfile(profileFlag, lookupEnv))
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("could not reach the server at %s: %w", serverURL, err))
		}
		matches := updates.CurrentVersion == report.CLI.Version
		report.Server = &updates
		report.ServerMatches = &matches
	}

	if err := renderResult(stdout, of.Format, of.Query, report, func() { printVersionHuman(stdout, report) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// localVersionInfo reads the ldflags version and the VCS stamp Go embeds at build time.
func localVersionInfo() cliVersionInfo {
	info := cliVersionInfo{Version: version.Version, Commit: unknownBuildInfo, BuildDate: unknownBuildInfo}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			info.Commit = s.Value
		case "vcs.time":
			info.BuildDate = s.Value
		}
	}
	if len(info.Commit) > shortCommitLen {
		info.Commit = info.Commit[:shortCommitLen]
	}
	return info
}

func printVersionHuman(out io.Writer, r versionReport) {
	_, _ = fmt.Fprintf(out, "cli version:      %s\n", r.CLI.Version)
	_, _ = fmt.Fprintf(out, "commit:           %s\n", r.CLI.Commit)
	_, _ = fmt.Fprintf(out, "built:            %s\n", r.CLI.BuildDate)
	if r.Server == nil {
		return
	}
	_, _ = fmt.Fprintln(out)
	printUpdatesHuman(out, *r.Server)
	match := "no"
	if r.ServerMatches != nil && *r.ServerMatches {
		match = "yes"
	}
	_, _ = fmt.Fprintf(out, "matches cli:       %s\n", match)
}

func versionUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s version [flags]

Shows this CLI's version, commit and build date. Runs offline: no server,
login or config is needed.

With --server, also asks the control plane for its running version, whether
it matches this CLI, and whether a newer release has been published to
github.com/glincker/levelrail. Fails if the server cannot be reached.

Flags:
  --server                 also query the control plane (opt-in)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string         control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string         named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                   print version info as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
