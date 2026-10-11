package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/rollback"
	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/upgradehistory"
	"github.com/GLINCKER/levelrail/internal/version"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

const (
	probeTimeout        = 15 * time.Second
	migrateCheckTimeout = 5 * time.Minute
	selfUpgradeWorkDir  = "self-upgrade-work"
	versionUsageMarker  = "version [--json]"
	migrateCheckMarker  = "migrate-check --db"
	releaseNotesLimit   = 50
	cosignOIDCIssuer    = "https://token.actions.githubusercontent.com"
	envReleaseBaseURL   = "LEVELRAIL_RELEASE_BASE_URL"
	envInsecureMirror   = "LEVELRAIL_INSECURE_MIRROR"
)

var initiatorPattern = regexp.MustCompile(`^[A-Za-z0-9._@:-]{1,64}$`)

type selfUpgradeFlags struct {
	to, ack, verify, initiator, repo, baseURL string
	latest, dryRun, asJSON, yes, recoverRun   bool
	skipNotes                                 bool
	stopCmd, startCmd, healthURL              string
	timeout                                   time.Duration
}

const selfUpgradeUsage = `usage: sudo levelrail self-upgrade --to <version> | --latest [flags]

Replaces the installed control plane with a newer release: downloads it, checks
the checksum and the cosign signature, snapshots the database, dry-runs the
migrations on a copy, swaps the binary, restarts and waits for health. If the
new release does not become healthy in time the previous binary and the data
come back automatically. Run it on the host as root.

  --to VERSION         release to install (must be newer than the running one)
  --latest             install the newest release on the configured channel
  --ack IDS            comma-separated breaking-change ids you acknowledge
  --dry-run            print the plan and breaking changes, change nothing
  --yes                skip the typed confirmation
  --json               machine-readable plan or result
  --verify MODE        auto (default), require or off, for the release signature
  --timeout DURATION   how long the new release may take to become healthy
  --recover            finish or undo an upgrade that was interrupted
  --skip-notes-check   continue when release notes cannot be fetched
  --initiator NAME     who asked, recorded in upgrade history
  --stop-cmd / --start-cmd / --health-url   same meaning as for rollback
`

// runSelfUpgrade implements "levelrail self-upgrade".
func runSelfUpgrade(ctx context.Context, args []string, dataDir string, stdin io.Reader, stdout io.Writer) error {
	f, err := parseSelfUpgradeFlags(args, stdout)
	if err != nil {
		return err
	}
	exe, err := executablePath()
	if err != nil {
		return err
	}
	name := filepath.Base(exe)
	live := filepath.Join(dataDir, storeFilename)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	journal := selfupgrade.NewJournal(dataDir)
	healthBase := f.healthURL
	if healthBase == "" {
		healthBase = "http://" + dashboardDialAddr(serviceHTTPAddr(ctx, name))
	}
	deps := selfUpgradeDeps(f, exe, name, dataDir, live, healthBase, journal, logger)

	if f.recoverRun {
		return recoverSelfUpgrade(ctx, deps, journal, f, stdout)
	}
	if running, _ := journal.Running(); len(running) > 0 {
		return fmt.Errorf("an earlier upgrade (%s, %s to %s) did not finish. Run: sudo %s self-upgrade --recover", running[0].ID, running[0].FromVersion, running[0].ToVersion, name)
	}

	repo, base, err := releaseSource(f)
	if err != nil {
		return err
	}
	target, err := resolveTarget(ctx, f, repo)
	if err != nil {
		return err
	}
	breaking, notesErr := breakingFor(ctx, repo, version.Version, target)
	if notesErr != nil && !f.skipNotes {
		return fmt.Errorf("could not read release notes to check for breaking changes: %w. Re-run with --skip-notes-check to continue without that check", notesErr)
	}
	plan := selfUpgradePlan{Current: version.Version, Target: target, Breaking: breaking}
	if f.dryRun {
		return printSelfUpgradePlan(stdout, plan, f.asJSON)
	}
	acked := splitIDs(f.ack)
	if missing := selfupgrade.MissingAcks(breaking, acked); len(missing) > 0 {
		_ = printSelfUpgradePlan(stdout, plan, f.asJSON)
		return selfupgrade.AckError(missing)
	}
	if !f.yes {
		_, _ = fmt.Fprintf(stdout, "Upgrading %s to %s. Type %s to confirm: ", version.Version, target, target)
		answer, _ := bufio.NewReader(stdin).ReadString('\n')
		if strings.TrimSpace(answer) != target {
			return errors.New("confirmation does not match the target version, nothing changed")
		}
	}

	// A dropped SSH session must not kill the command between stop and start.
	signal.Ignore(syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	if !f.asJSON {
		_, _ = fmt.Fprintln(stdout, "Applying. Do not close this session: interrupts are ignored until it finishes.")
	}
	deps.Fetch = selfupgrade.Fetcher{BaseURL: base, AllowHTTP: os.Getenv(envInsecureMirror) == "1"}.Fetch
	deps.Asset = fmt.Sprintf("%s-linux-%s", name, runtime.GOARCH)
	deps.VerifySig = selfupgrade.Signer{
		Mode:           f.verify,
		IdentityRegexp: "(?i)https://github.com/" + regexp.QuoteMeta(repo) + `/\.github/workflows/release\.yml@.*`,
		Issuer:         cosignOIDCIssuer,
	}.Verify
	deps.WorkDir = filepath.Join(dataDir, selfUpgradeWorkDir)

	if err := retainRunning(exe, name); err != nil {
		logger.Warn("running release not retained for rollback", slog.String("error", err.Error()))
	}
	attempt, applyErr := selfupgrade.Apply(ctx, deps, selfupgrade.Request{
		ID: newAttemptID(), CurrentVersion: version.Version, TargetVersion: target, Initiator: f.initiator,
		Breaking: breaking, Acked: acked, HealthTimeout: f.timeout,
	})
	recordAttempt(ctx, live, journal, logger)
	if attempt.Outcome == selfupgrade.OutcomeSucceeded {
		retainNew(exe, name, target, attempt.ToSchema, logger)
	}
	printAttempt(stdout, attempt, f.asJSON)
	return applyErr
}

type selfUpgradePlan struct {
	Current  string                 `json:"current_version"`
	Target   string                 `json:"target_version"`
	Breaking []selfupgrade.Breaking `json:"breaking"`
}

func printSelfUpgradePlan(out io.Writer, p selfUpgradePlan, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(out).Encode(p)
	}
	_, _ = fmt.Fprintf(out, "upgrade %s -> %s\n", p.Current, p.Target)
	if len(p.Breaking) == 0 {
		_, _ = fmt.Fprintln(out, "breaking changes: none declared")
	}
	for _, b := range p.Breaking {
		mark := "info"
		if b.RequiresAck {
			mark = "ACK REQUIRED"
		}
		_, _ = fmt.Fprintf(out, "breaking [%s] %s (%s): %s\n", mark, b.ID, b.Version, b.Summary)
	}
	_, _ = fmt.Fprintln(out, "steps: download, verify checksum, verify signature, snapshot, migration dry-run, stop, swap, start, health check (automatic rollback on failure)")
	return nil
}

func printAttempt(out io.Writer, a selfupgrade.Attempt, asJSON bool) {
	if asJSON {
		_ = json.NewEncoder(out).Encode(a)
		return
	}
	for _, s := range a.Steps {
		_, _ = fmt.Fprintf(out, "  [%s] %s %s\n", s.Status, s.Name, s.Detail)
	}
	_, _ = fmt.Fprintf(out, "%s: %s -> %s (backup %s)\n", a.Outcome, a.FromVersion, a.ToVersion, a.BackupName)
}

func recoverSelfUpgrade(ctx context.Context, deps selfupgrade.Deps, journal *selfupgrade.Journal, f selfUpgradeFlags, stdout io.Writer) error {
	running, err := journal.Running()
	if err != nil {
		return err
	}
	if len(running) == 0 {
		_, _ = fmt.Fprintln(stdout, "no interrupted upgrade found")
		return nil
	}
	var firstErr error
	for _, a := range running {
		got, rerr := selfupgrade.Recover(ctx, deps, a, f.timeout)
		printAttempt(stdout, got, f.asJSON)
		if rerr != nil && firstErr == nil {
			firstErr = rerr
		}
	}
	recordAttempt(ctx, filepath.Join(dataDirFromEnv(), storeFilename), journal, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	return firstErr
}

func parseSelfUpgradeFlags(args []string, stdout io.Writer) (selfUpgradeFlags, error) {
	var f selfUpgradeFlags
	fs := flag.NewFlagSet("self-upgrade", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&f.to, "to", "", "")
	fs.BoolVar(&f.latest, "latest", false, "")
	fs.StringVar(&f.ack, "ack", "", "")
	fs.StringVar(&f.verify, "verify", "", "")
	fs.StringVar(&f.initiator, "initiator", "", "")
	fs.StringVar(&f.repo, "repo", "", "")
	fs.StringVar(&f.baseURL, "base-url", "", "")
	fs.BoolVar(&f.dryRun, "dry-run", false, "")
	fs.BoolVar(&f.asJSON, "json", false, "")
	fs.BoolVar(&f.yes, "yes", false, "")
	fs.BoolVar(&f.recoverRun, "recover", false, "")
	fs.BoolVar(&f.skipNotes, "skip-notes-check", false, "")
	fs.StringVar(&f.stopCmd, "stop-cmd", "", "")
	fs.StringVar(&f.startCmd, "start-cmd", "", "")
	fs.StringVar(&f.healthURL, "health-url", "", "")
	fs.DurationVar(&f.timeout, "timeout", selfupgrade.DefaultHealthTimeout, "")
	if err := fs.Parse(args); err != nil || (!f.recoverRun && f.to == "" && !f.latest) {
		_, _ = fmt.Fprint(stdout, selfUpgradeUsage)
		return f, errors.New("invalid flags")
	}
	if f.to != "" && !selfupgrade.ValidTag(f.to) {
		return f, fmt.Errorf("%q is not a release tag such as v1.2.3", f.to)
	}
	if f.verify == "" {
		f.verify = os.Getenv("APP_INSTALL_VERIFY")
	}
	switch f.verify {
	case "":
		f.verify = selfupgrade.VerifyAuto
	case selfupgrade.VerifyAuto, selfupgrade.VerifyRequire, selfupgrade.VerifyOff:
	default:
		return f, fmt.Errorf("--verify must be auto, require or off, got %q", f.verify)
	}
	if f.initiator == "" {
		f.initiator = hostActor()
	}
	if !initiatorPattern.MatchString(f.initiator) {
		return f, errors.New("--initiator may only contain letters, digits and ._@:-")
	}
	return f, nil
}

func releaseSource(f selfUpgradeFlags) (repo, base string, err error) {
	repo = f.repo
	if repo == "" {
		b, berr := loadBrand()
		if berr != nil {
			return "", "", fmt.Errorf("cannot tell which repository publishes releases: %w (pass --repo owner/name)", berr)
		}
		repo = b.RepoSlug()
	}
	if repo == "" {
		return "", "", errors.New("no release repository configured (pass --repo owner/name)")
	}
	base = f.baseURL
	if base == "" {
		base = os.Getenv(envReleaseBaseURL)
	}
	if base == "" {
		base = "https://github.com/" + repo + "/releases/download"
	}
	return repo, base, nil
}

func resolveTarget(ctx context.Context, f selfUpgradeFlags, repo string) (string, error) {
	if f.to != "" {
		return f.to, nil
	}
	rel, err := upgrade.FetchLatestStable(ctx, repo)
	if err != nil {
		return "", fmt.Errorf("find the latest release: %w", err)
	}
	if rel == nil {
		return "", errors.New("no release has been published")
	}
	return rel.Tag, nil
}

func breakingFor(ctx context.Context, repo, current, target string) ([]selfupgrade.Breaking, error) {
	hist, err := upgrade.FetchHistory(ctx, repo, "all", releaseNotesLimit)
	if err != nil {
		return nil, err
	}
	notes := make([]selfupgrade.ReleaseNotes, 0, len(hist))
	for _, h := range hist {
		notes = append(notes, selfupgrade.ReleaseNotes{Tag: h.Tag, Body: h.Body})
	}
	return selfupgrade.BreakingBetween(notes, current, target), nil
}

func splitIDs(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func newAttemptID() string {
	id, err := store.NewAuditEntryID()
	if err != nil {
		return fmt.Sprintf("su-%d", time.Now().UnixNano())
	}
	return "su-" + strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(id))
}

func retainRunning(exe, name string) error {
	if !rollback.ValidVersion(version.Version) {
		return nil
	}
	own, err := currentVersionInfo()
	if err != nil {
		return err
	}
	_, err = rollback.Retain(rollback.Dir(exe), name, version.Version, exe, own.SchemaVersion, time.Now())
	return err
}

func retainNew(exe, name, target string, schema int, logger *slog.Logger) {
	if !rollback.ValidVersion(target) {
		return
	}
	if _, err := rollback.Retain(rollback.Dir(exe), name, target, exe, schema, time.Now()); err != nil {
		logger.Warn("new release not retained for rollback", slog.String("error", err.Error()))
	}
}

// importSelfUpgradeJournal records attempts a host command journaled while
// the database was being swapped or the service was down.
func importSelfUpgradeJournal(ctx context.Context, db *store.DB, logger *slog.Logger) {
	n, err := selfupgrade.ImportJournal(ctx, selfupgrade.NewJournal(dataDirFromEnv()), db)
	if err != nil {
		logger.Warn("self-upgrade history import failed", slog.String("error", err.Error()))
		return
	}
	if n > 0 {
		logger.Info("self-upgrade attempts recorded", slog.Int("attempts", n))
	}
}

// recordAttempt copies the journal into the database. It is best effort: the
// control plane imports it again at its next boot.
func recordAttempt(ctx context.Context, live string, journal *selfupgrade.Journal, logger *slog.Logger) {
	db, err := store.Open(ctx, live)
	if err != nil {
		logger.Warn("self-upgrade history deferred to next boot", slog.String("error", err.Error()))
		return
	}
	defer func() { _ = db.Close() }()
	if _, err := selfupgrade.ImportJournal(ctx, journal, db); err != nil {
		logger.Warn("self-upgrade history not stored", slog.String("error", err.Error()))
	}
}

func selfUpgradeDeps(f selfUpgradeFlags, exe, name, dataDir, live, healthBase string, journal *selfupgrade.Journal, logger *slog.Logger) selfupgrade.Deps {
	service := func(ctx context.Context, custom, verb string) error {
		var cmd *exec.Cmd
		if custom != "" {
			argv, err := splitCommand(custom)
			if err != nil {
				return fmt.Errorf("%s: %w", verb, err)
			}
			cmd = exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // operator-supplied flag on a root-run host command, run as argv with no shell
		} else {
			cmd = exec.CommandContext(ctx, "systemctl", verb, name) //nolint:gosec // name is this executable's own file name
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w: %s", verb, err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	return selfupgrade.Deps{
		BinaryPath: exe, Journal: journal, Log: logger, Now: time.Now,
		Probe:    probeBinary,
		DBSchema: func(ctx context.Context) (int, error) { return rollback.DBSchemaVersion(ctx, live) },
		Backup: func(ctx context.Context) (selfupgrade.BackupInfo, error) {
			db, err := store.Open(ctx, live)
			if err != nil {
				return selfupgrade.BackupInfo{}, fmt.Errorf("open database for backup: %w", err)
			}
			defer func() { _ = db.Close() }()
			info, err := cpbackup.NewManager(db, dataDir).Create(ctx)
			if err != nil {
				return selfupgrade.BackupInfo{}, err
			}
			return selfupgrade.BackupInfo{Name: info.Name, SnapshotPath: filepath.Join(dataDir, cpbackup.DirName, info.Name)}, nil
		},
		MigrateTest: func(ctx context.Context, binary, snapshot string) (int, error) {
			return migrateCheckWith(ctx, binary, snapshot, dataDir)
		},
		Stop:        func(ctx context.Context) error { return service(ctx, f.stopCmd, "stop") },
		Start:       func(ctx context.Context) error { return service(ctx, f.startCmd, "start") },
		WaitHealthy: func(ctx context.Context, t time.Duration) error { return rollback.WaitHealthy(ctx, healthBase, t) },
		RestoreDB: func(_ context.Context, snapshot string) (string, error) {
			return rollback.SwapInBackup(live, snapshot, time.Now())
		},
		WriteMarker: func(_ context.Context, a selfupgrade.Attempt) error {
			return upgradehistory.WriteMarker(dataDir, upgradehistory.Marker{
				ToVersion: a.ToVersion, Initiator: f.initiator, Method: upgradehistory.MethodSelfUpgrade,
				BackupName: a.BackupName, Reason: "self-upgrade " + a.ID, WrittenAt: time.Now().UTC(),
			})
		},
		ClearMarker: func() { upgradehistory.ClearMarker(dataDir) },
		Audit: func(ctx context.Context, detail string) error {
			return rollback.WriteAudit(ctx, live, f.initiator, "host self-upgrade: "+detail, time.Now())
		},
	}
}

// isolatedEnv keeps a probe or dry-run of an untrusted-until-verified binary
// away from the live data directory and the live listeners, so an old release
// that treats an unknown argument as "start serving" cannot touch real state.
func isolatedEnv(scratch string) []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + scratch,
		"APP_DATA_DIR=" + scratch,
		"APP_HTTP_ADDR=127.0.0.1:0",
		"APP_INGRESS_HTTP_ADDR=127.0.0.1:0",
		"APP_INGRESS_HTTPS_ADDR=127.0.0.1:0",
	}
}

// lastLine returns the final non-empty line: a library may log to stdout
// before the command prints its JSON result.
func lastLine(b []byte) []byte {
	lines := bytes.Split(bytes.TrimSpace(b), []byte("\n"))
	return bytes.TrimSpace(lines[len(lines)-1])
}

func binaryContains(path, marker string) bool {
	raw, err := os.ReadFile(path) //nolint:gosec // downloaded into this command's own work directory
	return err == nil && bytes.Contains(raw, []byte(marker))
}

func probeBinary(ctx context.Context, path string) (selfupgrade.ProbeInfo, error) {
	if !binaryContains(path, versionUsageMarker) {
		return selfupgrade.ProbeInfo{}, errors.New("binary has no version subcommand, so it cannot be probed safely; use the installer for this release")
	}
	scratch, err := os.MkdirTemp("", "probe-")
	if err != nil {
		return selfupgrade.ProbeInfo{}, fmt.Errorf("create probe scratch directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "version", "--json") //nolint:gosec // checksum-verified binary in the work directory
	cmd.Env = isolatedEnv(scratch)
	out, err := cmd.Output()
	if err != nil {
		return selfupgrade.ProbeInfo{}, fmt.Errorf("run downloaded binary: %w", err)
	}
	var info versionInfo
	if err := json.Unmarshal(lastLine(out), &info); err != nil || info.Version == "" {
		return selfupgrade.ProbeInfo{}, errors.New("downloaded binary did not report a version")
	}
	return selfupgrade.ProbeInfo{Version: info.Version, Schema: info.SchemaVersion}, nil
}

type migrateCheckResult struct {
	SchemaBefore int `json:"schema_before"`
	SchemaAfter  int `json:"schema_after"`
}

func migrateCheckWith(ctx context.Context, binary, snapshot, dataDir string) (int, error) {
	if !binaryContains(binary, migrateCheckMarker) {
		return -1, selfupgrade.ErrDryRunUnsupported
	}
	scratch, err := os.MkdirTemp(dataDir, "migrate-check-")
	if err != nil {
		return -1, fmt.Errorf("create scratch directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	copyPath := filepath.Join(scratch, storeFilename)
	if err := copyFile(snapshot, copyPath); err != nil {
		return -1, fmt.Errorf("copy snapshot for the dry run: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, migrateCheckTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "migrate-check", "--db", copyPath, "--json") //nolint:gosec // checksum-verified binary in the work directory
	cmd.Env = isolatedEnv(scratch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return -1, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var res migrateCheckResult
	if err := json.Unmarshal(lastLine(out), &res); err != nil {
		return -1, fmt.Errorf("unreadable migrate-check output: %q", strings.TrimSpace(string(out)))
	}
	return res.SchemaAfter, nil
}

// runMigrateCheck implements "levelrail migrate-check --db FILE [--json]": it
// applies this binary's migrations to a copy and reports the schema reached.
// It never touches the live data directory.
func runMigrateCheck(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("migrate-check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	path := fs.String("db", "", "")
	asJSON := fs.Bool("json", false, "")
	if err := fs.Parse(args); err != nil || *path == "" {
		return errors.New("usage: levelrail migrate-check --db <database copy> [--json]")
	}
	state, err := store.ReadSchemaState(ctx, *path)
	if err != nil {
		return err
	}
	db, err := store.Open(ctx, *path)
	if err != nil {
		return fmt.Errorf("apply migrations to the copy: %w", err)
	}
	defer func() { _ = db.Close() }()
	after, err := rollback.DBSchemaVersion(ctx, *path)
	if err != nil {
		return err
	}
	res := migrateCheckResult{SchemaBefore: state.Schema, SchemaAfter: after}
	if *asJSON {
		return json.NewEncoder(stdout).Encode(res)
	}
	_, err = fmt.Fprintf(stdout, "migrates from schema %d to %d\n", res.SchemaBefore, res.SchemaAfter)
	return err
}

// downgradeRefusal is returned by run when the database is newer than this
// binary; main prints the message and exits with ExitDowngradeRefused.
type downgradeRefusal struct{ msg string }

func (e *downgradeRefusal) Error() string { return e.msg }

func newDowngradeRefusal(ctx context.Context, dataDir string) *downgradeRefusal {
	in := selfupgrade.GuardInput{BinaryVersion: version.Version, Program: "levelrail", InstallerCommand: installerPrefix()}
	if exe, err := os.Executable(); err == nil {
		in.Program = filepath.Base(exe)
	}
	if newest, err := store.MaxSchemaVersion(); err == nil {
		in.BinarySchema = newest
	}
	if st, err := store.ReadSchemaState(ctx, filepath.Join(dataDir, storeFilename)); err == nil {
		in.DBSchema, in.DBVersion = st.Schema, st.LastVersion
	}
	return &downgradeRefusal{msg: selfupgrade.DowngradeMessage(in)}
}

func installerPrefix() string {
	slug := "glincker/levelrail"
	if b, err := loadBrand(); err == nil && b.RepoSlug() != "" {
		slug = b.RepoSlug()
	}
	return "curl -fsSL https://raw.githubusercontent.com/" + slug + "/main/install.sh | sudo"
}
