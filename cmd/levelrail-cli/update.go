package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/cliupdate"
)

const (
	envReleaseAPIURL    = "LEVELRAIL_RELEASE_API_URL"
	envReleaseBaseURL   = "LEVELRAIL_RELEASE_BASE_URL"
	envChannel          = "LEVELRAIL_CHANNEL"
	envInsecureMirror   = "LEVELRAIL_INSECURE_MIRROR"
	envNoUpdateCheck    = "LEVELRAIL_NO_UPDATE_CHECK"
	envUpdateCheckDays  = "LEVELRAIL_UPDATE_CHECK_DAYS"
	envUpdateHintWaitMS = "LEVELRAIL_UPDATE_HINT_WAIT_MS"

	defaultReleaseAPIURL    = "https://api.github.com"
	defaultUpdateCheckDays  = 7
	defaultUpdateHintWaitMS = 750
	updateHTTPTimeout       = 10 * time.Minute
	tufCacheDirName         = "sigstore-tuf"
)

// Seams for tests: the running executable's path and the signature check.
var (
	updateExePath = cliupdate.ExecutablePath
	updateVerify  func(ctx context.Context, checksumsPath, bundlePath string) (string, error)
)

type updateCheckReport struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
}

// runUpdate checks for a newer CLI release, or replaces this executable with a verified one.
func runUpdate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" update", flag.ContinueOnError)
	fs.SetOutput(stderr)
	check := fs.Bool("check", false, "only report whether a newer release exists, change nothing")
	target := fs.String("version", "", "release tag to install, also allows a downgrade (default: latest)")
	dryRun := fs.Bool("dry-run", false, "show what would change without downloading or replacing anything")
	channel := fs.String("channel", "", "release channel: stable or beta (default: $"+envChannel+", else stable with a prerelease fallback)")
	exitCode := fs.Bool("exit-code", false, "with --check, exit 10 when an update is available")
	force := fs.Bool("force", false, "replace the binary even when a package manager owns it")
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	jsonOut := fs.Bool("json", false, "print the result as JSON to stdout and nothing else")
	outputFlag, queryFlag := bindOutputQueryFlags(fs)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, updateUsage(prog)) }
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
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "%s: update takes no arguments, got %q\n", prog, fs.Arg(0))
		return exitUsage
	}
	src, err := updateSource(lookupEnv)
	if err != nil {
		return reportError(stdout, stderr, *jsonOut, err)
	}
	if *channel != "" {
		src.Channel = *channel
	}
	current := localVersionInfo().Version

	if *check {
		latest, err := src.Latest(context.Background())
		if err != nil {
			return reportError(stdout, stderr, *jsonOut, fmt.Errorf("check for updates: %w", err))
		}
		report := updateCheckReport{Current: current, Latest: latest, UpdateAvailable: cliupdate.Newer(current, latest)}
		code := renderUpdate(stdout, stderr, format, *queryFlag, report, func() { printUpdateCheck(stdout, prog, report) })
		if code == exitOK && *exitCode && report.UpdateAvailable {
			return exitUpdateAvail
		}
		return code
	}

	exe, err := updateExePath()
	if err != nil {
		return reportError(stdout, stderr, *jsonOut, err)
	}
	opts := cliupdate.Options{
		Source: src, Current: current, Target: *target, DryRun: *dryRun, Force: *force, ExePath: exe,
		Verify: updateVerify,
	}
	if dir, derr := configDir(prog); derr == nil {
		opts.TUFCacheDir = filepath.Join(dir, tufCacheDirName)
	}
	if !*yes {
		opts.Confirm = func(cur, tgt string) error { return confirmUpdate(stderr, prog, cur, tgt) }
	}
	res, err := cliupdate.Update(context.Background(), opts)
	if err != nil {
		return reportError(stdout, stderr, *jsonOut, fmt.Errorf("update: %w", err))
	}
	return renderUpdate(stdout, stderr, format, *queryFlag, res, func() { printUpdateResult(stdout, res) })
}

func renderUpdate(stdout, stderr io.Writer, format outputFormat, query string, data any, table func()) int {
	if err := renderResult(stdout, format, query, data, table); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// updateSource resolves the release repo and asset for this build from the brand config and env.
func updateSource(lookupEnv func(string) (string, bool)) (cliupdate.Source, error) {
	b, err := brand.LoadEmbedded()
	if err != nil {
		return cliupdate.Source{}, fmt.Errorf("load release settings: %w", err)
	}
	repo := b.RepoSlug()
	if repo == "" {
		return cliupdate.Source{}, errors.New("no release repository is configured")
	}
	env := func(k, def string) string {
		if v, ok := lookupEnv(k); ok && v != "" {
			return v
		}
		return def
	}
	insecure, _ := lookupEnv(envInsecureMirror)
	return cliupdate.Source{
		Channel:      env(envChannel, ""),
		Repo:         repo,
		APIBase:      env(envReleaseAPIURL, defaultReleaseAPIURL),
		DownloadBase: env(envReleaseBaseURL, "https://github.com/"+repo+"/releases/download"),
		Asset:        cliupdate.Asset(b.BinaryName+"-cli", runtime.GOOS, runtime.GOARCH),
		Client:       &http.Client{Timeout: updateHTTPTimeout},
		AllowHTTP:    insecure == "1",
	}, nil
}

func confirmUpdate(stderr io.Writer, prog, current, target string) error {
	if ciMode {
		return newValidationError("%s update needs confirmation but CI mode never prompts: pass --yes", prog)
	}
	_, _ = fmt.Fprintf(stderr, "Replace %s %s with %s? [y/N] ", prog, current, target)
	line, _ := bufio.NewReader(stdinSource).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		return errors.New("cancelled, nothing changed")
	}
	return nil
}

func printUpdateCheck(out io.Writer, prog string, r updateCheckReport) {
	_, _ = fmt.Fprintf(out, "current: %s\n", r.Current)
	latest := r.Latest
	if latest == "" {
		latest = "none published"
	}
	_, _ = fmt.Fprintf(out, "latest:  %s\n", latest)
	switch {
	case r.UpdateAvailable:
		_, _ = fmt.Fprintf(out, "an update is available. Run: %s update\n", prog)
	case r.Current == "dev":
		_, _ = fmt.Fprintln(out, "this is a development build, so it is not compared with releases")
	default:
		_, _ = fmt.Fprintln(out, "up to date")
	}
}

func printUpdateResult(out io.Writer, r cliupdate.Result) {
	switch r.Action {
	case cliupdate.ActionUpToDate:
		_, _ = fmt.Fprintf(out, "already at %s, nothing to do\n", r.Current)
	case cliupdate.ActionWouldApply:
		_, _ = fmt.Fprintf(out, "would replace %s: %s to %s (asset %s). Nothing was changed.\n", r.Path, r.Current, r.Target, r.Asset)
	default:
		_, _ = fmt.Fprintf(out, "updated %s: %s to %s\nverified: %s\n", r.Path, r.Current, r.Target, r.Verification)
		if r.Backup != "" {
			_, _ = fmt.Fprintf(out, "previous binary kept at %s. Roll back with: mv %s %s\n", r.Backup, r.Backup, r.Path)
		}
	}
}

// updateIntervalDays reads the hint interval from env; 0 or less disables it.
func updateIntervalDays(lookupEnv func(string) (string, bool)) int {
	if v, ok := lookupEnv(envUpdateCheckDays); ok && strings.TrimSpace(v) != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return defaultUpdateCheckDays
}

func updateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s update --check [--exit-code] [flags]   (self-update is an alias)
  %[1]s update [--version TAG] [--dry-run] [--yes] [flags]

--check reports the current version, the latest release and whether an update
is available. It always exits 0 when the check itself worked, even if you are
out of date.

Without --check, downloads the release for this OS and arch, verifies its
checksum and Sigstore signature, then replaces this executable. If the
signature or checksum cannot be verified, nothing is changed. The new binary
is run once (version only) before the swap. Needs write access to the directory
holding this executable: if it is managed by a package manager or owned by
root, update it that way or re-run with sudo.

Flags:
  --check                  only report whether a newer release exists
  --version string         release tag to install, also allows a downgrade (default: latest)
  --dry-run                show what would change without downloading or replacing anything
  --channel string         stable or beta (default: $LEVELRAIL_CHANNEL, else stable with a prerelease fallback)
  --exit-code              with --check, exit 10 when an update is available (0 when current)
  --force                  replace the binary even when a package manager owns it
  --yes                    do not ask for confirmation (required in CI mode or without a terminal)
  --json                   print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help

Environment:
  %[2]s   releases API base (default %[3]s)
  %[4]s  asset download base (default: the release repo's GitHub releases)
  %[5]s        days between automatic "newer release" hints, 0 disables (default %[6]d)
  %[7]s         set to 1 to turn the hint off
`, prog, envReleaseAPIURL, defaultReleaseAPIURL, envReleaseBaseURL, envUpdateCheckDays, defaultUpdateCheckDays, envNoUpdateCheck)
}
