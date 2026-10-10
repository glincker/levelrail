package main

import (
	"bufio"
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
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/rollback"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/upgradehistory"
	"github.com/GLINCKER/levelrail/internal/version"
)

const defaultRollbackHealthTimeout = 90 * time.Second

type versionInfo struct {
	Version       string `json:"version"`
	SchemaVersion int    `json:"schema_version"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
}

func currentVersionInfo() (versionInfo, error) {
	schema, err := store.MaxSchemaVersion()
	if err != nil {
		return versionInfo{}, fmt.Errorf("read schema version: %w", err)
	}
	return versionInfo{Version: version.Version, SchemaVersion: schema, OS: runtime.GOOS, Arch: runtime.GOARCH}, nil
}

// runVersion implements "levelrail version [--json]". The release workflow
// publishes its --json output as the release manifest.
func runVersion(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	asJSON := fs.Bool("json", false, "")
	if err := fs.Parse(args); err != nil {
		return errors.New("usage: levelrail version [--json]")
	}
	info, err := currentVersionInfo()
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(stdout).Encode(info)
	}
	_, err = fmt.Fprintf(stdout, "%s (schema %d, %s/%s)\n", info.Version, info.SchemaVersion, info.OS, info.Arch)
	return err
}

func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate own executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// runRetain implements "levelrail retain --version V --file F": the installer
// calls it, with the new binary, to keep release binaries for rollback.
func runRetain(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("retain", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	ver := fs.String("version", "", "release tag of the binary")
	file := fs.String("file", "", "binary to keep")
	schema := fs.Int("schema-version", -1, "schema version the binary supports (-1 unknown)")
	dir := fs.String("releases-dir", "", "retained releases directory (default: beside the executable)")
	if err := fs.Parse(args); err != nil {
		return errors.New("usage: levelrail retain --version <tag> --file <binary> [--schema-version N] [--releases-dir DIR]")
	}
	if *ver == "" || *file == "" {
		return errors.New("usage: levelrail retain --version <tag> --file <binary> [--schema-version N] [--releases-dir DIR]")
	}
	exe, err := executablePath()
	if err != nil {
		return err
	}
	releases := *dir
	if releases == "" {
		releases = rollback.Dir(exe)
	}
	if *file == exe {
		own, err := currentVersionInfo()
		if err != nil {
			return err
		}
		*schema = own.SchemaVersion
	}
	if *schema < 0 {
		if prev, ok, err := rollback.Find(releases, filepath.Base(exe), *ver); err == nil && ok {
			*schema = prev.SchemaVersion
		}
	}
	r, err := rollback.Retain(releases, filepath.Base(exe), *ver, *file, *schema, time.Now())
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "retained %s (%d bytes, sha256 %s) in %s\n", r.Version, r.SizeBytes, r.SHA256, releases)
	return nil
}

type rollbackFlags struct {
	to, confirm, restore, confirmLoss string
	acceptUnknown, dryRun, asJSON     bool
	stopCmd, startCmd, healthURL      string
	releasesDir                       string
	timeout                           time.Duration
}

const rollbackUsage = `usage: sudo levelrail rollback --to <version> [flags]

Replaces the installed control plane binary with a retained earlier release.
Run it on the host as root. The control plane itself never does this.

  --to VERSION               release to return to (must be retained on this host)
  --confirm VERSION          type the version again to confirm (prompted if omitted)
  --dry-run                  print the plan and change nothing
  --json                     machine-readable plan or result
  --restore-backup NAME      only when the target is older than the database
                             schema: replace the database with this backup.
                             Everything written after the backup is lost.
  --confirm-data-loss NAME   repeat the backup name to confirm that loss
  --accept-unknown-schema    try a release whose schema version is unknown
  --stop-cmd / --start-cmd   commands to stop/start the service, run
                             directly as argv (no shell, no pipes)
                             (default: systemctl stop/start <binary name>)
  --health-url URL           base URL probed for /healthz and /readyz
  --timeout DURATION         how long the target may take to become healthy
  --releases-dir DIR         retained releases directory
`

// runRollback implements "levelrail rollback".
func runRollback(ctx context.Context, args []string, dataDir string, stdin io.Reader, stdout io.Writer) error {
	f, err := parseRollbackFlags(args, stdout)
	if err != nil {
		return err
	}
	exe, err := executablePath()
	if err != nil {
		return err
	}
	name := filepath.Base(exe)
	relDir := f.releasesDir
	if relDir == "" {
		relDir = rollback.Dir(exe)
	}
	live := filepath.Join(dataDir, storeFilename)
	dbSchema, err := rollback.DBSchemaVersion(ctx, live)
	if err != nil {
		return err
	}
	target, ok, err := rollback.Find(relDir, name, f.to)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s is not retained on this host (%s). Fetch it with the verified installer first: sudo LEVELRAIL_VERSION=%s sh install.sh rollback", f.to, relDir, f.to)
	}
	backups, err := rollback.ListBackups(ctx, dataDir)
	if err != nil {
		return err
	}
	var dbSize int64
	if st, err := os.Stat(live); err == nil {
		dbSize = st.Size()
	}
	plan := rollback.BuildPlan(rollback.PlanInput{
		CurrentVersion: version.Version, CurrentSchemaVersion: dbSchema, DBSizeBytes: dbSize, Backups: backups, ProgramName: name,
		Target: rollback.Target{Version: target.Version, SchemaVersion: target.SchemaVersion, SchemaSource: rollback.SourceRetained, Retained: true, SHA256: target.SHA256},
	})
	if f.dryRun {
		return printPlan(stdout, plan, f.asJSON)
	}
	if f.confirm == "" {
		_, _ = fmt.Fprintf(stdout, "Rolling back %s to %s. Type %s to confirm: ", version.Version, f.to, f.to)
		answer, _ := bufio.NewReader(stdin).ReadString('\n')
		f.confirm = strings.TrimSpace(answer)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	healthBase := f.healthURL
	if healthBase == "" {
		healthBase = "http://" + dashboardDialAddr(serviceHTTPAddr(ctx, name))
	}
	// A dropped SSH session must not kill the command between stop and start.
	signal.Ignore(syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	if !f.asJSON {
		_, _ = fmt.Fprintln(stdout, "Applying. Do not close this session: interrupts are ignored until it finishes.")
	}
	own, err := currentVersionInfo()
	if err != nil {
		return err
	}
	if rollback.ValidVersion(own.Version) {
		if _, err := rollback.Retain(relDir, name, own.Version, exe, own.SchemaVersion, time.Now()); err != nil {
			return fmt.Errorf("retain running release before rollback: %w", err)
		}
	}
	deps := hostDeps(f, exe, name, relDir, dataDir, live, healthBase, logger)
	res, err := rollback.Apply(ctx, deps, plan, rollback.Options{
		CurrentVersion: version.Version, ConfirmVersion: f.confirm, RestoreBackup: f.restore,
		ConfirmDataLoss: f.confirmLoss, AcceptUnknown: f.acceptUnknown, HealthTimeout: f.timeout,
	})
	if f.asJSON {
		_ = json.NewEncoder(stdout).Encode(res)
	} else if res.Outcome != "" {
		_, _ = fmt.Fprintf(stdout, "%s: %s -> %s (pre-rollback backup %s)\n", res.Outcome, res.From, res.To, res.BackupName)
	}
	return err
}

// serviceHTTPAddr reads APP_HTTP_ADDR from the service's own unit: a sudo shell
// does not carry it, so the process env alone would probe the default port.
func serviceHTTPAddr(ctx context.Context, name string) string {
	if v := os.Getenv("APP_HTTP_ADDR"); v != "" {
		return v
	}
	out, err := exec.CommandContext(ctx, "systemctl", "show", name, "--property=Environment", "--value").Output() //nolint:gosec // name is this executable's own file name
	if err == nil {
		if v := envFromSystemctl(string(out), "APP_HTTP_ADDR"); v != "" {
			return v
		}
	}
	return httpAddr()
}

func envFromSystemctl(out, key string) string {
	for _, field := range strings.Fields(out) {
		if v, ok := strings.CutPrefix(strings.Trim(field, `"`), key+"="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

func parseRollbackFlags(args []string, stdout io.Writer) (rollbackFlags, error) {
	var f rollbackFlags
	fs := flag.NewFlagSet("rollback", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&f.to, "to", "", "")
	fs.StringVar(&f.confirm, "confirm", "", "")
	fs.StringVar(&f.restore, "restore-backup", "", "")
	fs.StringVar(&f.confirmLoss, "confirm-data-loss", "", "")
	fs.BoolVar(&f.acceptUnknown, "accept-unknown-schema", false, "")
	fs.BoolVar(&f.dryRun, "dry-run", false, "")
	fs.BoolVar(&f.asJSON, "json", false, "")
	fs.StringVar(&f.stopCmd, "stop-cmd", "", "")
	fs.StringVar(&f.startCmd, "start-cmd", "", "")
	fs.StringVar(&f.healthURL, "health-url", "", "")
	fs.StringVar(&f.releasesDir, "releases-dir", "", "")
	fs.DurationVar(&f.timeout, "timeout", defaultRollbackHealthTimeout, "")
	if err := fs.Parse(args); err != nil || f.to == "" {
		_, _ = fmt.Fprint(stdout, rollbackUsage)
		return f, errors.New("invalid flags")
	}
	if !rollback.ValidVersion(f.to) {
		return f, fmt.Errorf("%w: %q", rollback.ErrInvalidVersion, f.to)
	}
	return f, nil
}

func printPlan(out io.Writer, p rollback.Plan, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(out).Encode(p)
	}
	_, _ = fmt.Fprintf(out, "rollback %s -> %s\n", p.CurrentVersion, p.Target.Version)
	_, _ = fmt.Fprintf(out, "database schema %d, target supports %d (%s)\n", p.CurrentSchemaVersion, p.Target.SchemaVersion, p.Verdict)
	_, _ = fmt.Fprintf(out, "steps: %s\nestimated downtime: about %ds\n", strings.Join(p.Steps, ", "), p.DowntimeSeconds)
	if p.RestoreRequired {
		_, _ = fmt.Fprintln(out, "WARNING: a binary-only rollback is refused. Restoring a backup loses data written after it.")
		for _, b := range p.Backups {
			_, _ = fmt.Fprintf(out, "  %s taken %s schema %d compatible=%v\n", b.Name, b.CreatedAt.Format(time.RFC3339), b.SchemaVersion, b.Compatible)
		}
	}
	_, _ = fmt.Fprintf(out, "apply: %s\n", p.Command)
	return nil
}

// splitCommand turns an operator's stop or start command into argv. It never
// goes through a shell, so quoting, pipes and && are not interpreted.
func splitCommand(s string) ([]string, error) {
	argv := strings.Fields(s)
	if len(argv) == 0 {
		return nil, errors.New("command is empty")
	}
	return argv, nil
}

func hostDeps(f rollbackFlags, exe, name, relDir, dataDir, live, healthBase string, logger *slog.Logger) rollback.Deps {
	run := func(ctx context.Context, custom, verb string) error {
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
	return rollback.Deps{
		BinaryPath: exe, ReleasesDir: relDir, BinaryName: name, Log: logger,
		DBSchema: func(ctx context.Context) (int, error) { return rollback.DBSchemaVersion(ctx, live) },
		Backups:  func(ctx context.Context) ([]rollback.BackupOption, error) { return rollback.ListBackups(ctx, dataDir) },
		Backup: func(ctx context.Context) (string, error) {
			db, err := store.Open(ctx, live)
			if err != nil {
				return "", fmt.Errorf("open database for backup: %w", err)
			}
			defer func() { _ = db.Close() }()
			info, err := cpbackup.NewManager(db, dataDir).Create(ctx)
			if err != nil {
				return "", err
			}
			return info.Name, nil
		},
		Stop:        func(ctx context.Context) error { return run(ctx, f.stopCmd, "stop") },
		Start:       func(ctx context.Context) error { return run(ctx, f.startCmd, "start") },
		WaitHealthy: func(ctx context.Context, t time.Duration) error { return rollback.WaitHealthy(ctx, healthBase, t) },
		RestoreDB: func(_ context.Context, backup string) (string, error) {
			if !cpbackup.ValidName(backup) {
				return "", cpbackup.ErrInvalidName
			}
			return rollback.SwapInBackup(live, filepath.Join(dataDir, cpbackup.DirName, backup), time.Now())
		},
		UndoRestoreDB: func(_ context.Context, kept string) error { return rollback.UndoSwap(live, kept) },
		Audit: func(ctx context.Context, detail string) error {
			return rollback.WriteAudit(ctx, live, hostActor(), "host rollback: "+detail, time.Now())
		},
		WriteMarker: func(_ context.Context, backup string) error {
			return upgradehistory.WriteMarker(dataDir, upgradehistory.Marker{
				ToVersion: f.to, Initiator: hostActor(), Method: upgradehistory.MethodRollback,
				BackupName: backup, WrittenAt: time.Now().UTC(),
			})
		},
		ClearMarker: func() { upgradehistory.ClearMarker(dataDir) },
		Now:         time.Now,
	}
}

func hostActor() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return "root"
}

// runReleaseCommand handles the host-side release subcommands and reports
// whether name was one of them.
func runReleaseCommand(name string, args []string) bool {
	var err error
	switch name {
	case "version":
		err = runVersion(args, os.Stdout)
	case "retain":
		err = runRetain(args, os.Stdout)
	case "rollback":
		err = runRollback(context.Background(), args, dataDirFromEnv(), os.Stdin, os.Stdout)
	default:
		return false
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	return true
}
