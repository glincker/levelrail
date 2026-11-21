// Package agentupdate replaces the running agent binary with a verified
// release, keeps the previous one as .prev and can swap it back.
package agentupdate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/nodeinsight"
	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/kit/semver"
)

// Outcomes reported in Result.Action.
const (
	ActionNone      = "none"
	ActionAvailable = "available"
	ActionStaged    = "staged"
	ActionRestarted = "restarted"
	ActionRolled    = "rolled-back"
	ActionHold      = "hold"
)

const restartTimeout = 30 * time.Second

var serviceNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]{0,63}$`)

// ErrNoServiceManager means no supported service manager owns this agent.
var ErrNoServiceManager = errors.New("no service manager found to restart the agent")

// Release fetches and verifies one release. The real implementation wraps
// selfupgrade.Fetcher and selfupgrade.Signer in require mode.
type Release interface {
	Fetch(ctx context.Context, tag, asset, dir string) (selfupgrade.Fetched, error)
	Verify(ctx context.Context, checksumsPath, bundlePath string) (string, error)
}

// Service restarts the agent through its service manager.
type Service interface {
	Restart(ctx context.Context) error
}

// Decider answers what the control plane wants this node to run.
type Decider func(ctx context.Context) (nodeinsight.UpdateDecision, error)

// Options configures one run.
type Options struct {
	Decide  Decider
	Release Release
	Service Service
	// Asset is the release asset for this OS and arch.
	Asset   string
	Current string
	// DataDir holds the install marker and the failed-target record.
	DataDir string
	ExePath string
	// Apply installs and restarts now; Check only reports. With neither the
	// update is installed only when the control plane says AutoApply.
	Apply bool
	Check bool
	// Smoke runs the staged binary and returns its version; required to install.
	Smoke func(ctx context.Context, path string) (string, error)
	// ForceTarget installs this release instead of the control plane answer. It
	// still goes through the signed download, version check and rollback.
	ForceTarget string
}

// Result describes what a run did.
type Result struct {
	Action   string `json:"action"`
	Current  string `json:"current"`
	Target   string `json:"target,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Previous string `json:"previous,omitempty"`
	Detail   string `json:"verification,omitempty"`
}

// IsReleaseTarget reports whether v is a published release tag such as v1.2.3,
// as opposed to dev or main-<sha> builds.
func IsReleaseTarget(v string) bool {
	if len(v) < 2 || v[0] != 'v' || v[1] < '0' || v[1] > '9' {
		return false
	}
	_, ok := semver.Compare(v, v)
	return ok
}

// ForceCommand is the one-line retry for a target the agent did not apply.
func ForceCommand(target string) string {
	return "levelrail-agent self-update --apply --force-target " + target
}

// PrevPath is where the replaced binary is kept.
func PrevPath(exe string) string { return exe + ".prev" }

// Run asks for the target and, when allowed, installs it.
func Run(ctx context.Context, o Options) (Result, error) {
	res := Result{Action: ActionNone, Current: o.Current}
	var d nodeinsight.UpdateDecision
	if o.ForceTarget != "" {
		d = nodeinsight.UpdateDecision{Action: nodeinsight.UpdateInstall, Target: o.ForceTarget, Reason: "target chosen with --force-target"}
	} else {
		var err error
		d, err = o.Decide(ctx)
		if err != nil {
			return res, fmt.Errorf("ask the control plane for the target version: %w", err)
		}
	}
	res.Reason = d.Reason
	if d.Action == nodeinsight.UpdateNone && !IsReleaseTarget(o.Current) {
		res.Reason += "; this is a development build, to move to a release run: " + ForceCommand("<release tag>")
	}
	if d.Action != nodeinsight.UpdateInstall && d.Action != nodeinsight.UpdateRollback {
		return res, nil
	}
	if d.Target == "" || d.Target == o.Current {
		return res, nil
	}
	if d.Target == "dev" || strings.HasPrefix(d.Target, "main-") {
		// Unpublished builds are never fetchable; skip without an error.
		res.Target = d.Target
		res.Reason = "target " + d.Target + " is not a release version; to pick a release run: " + ForceCommand("<release tag>")
		return res, nil
	}
	if !selfupgrade.ValidTag(d.Target) {
		return res, fmt.Errorf("refusing unsafe release tag %q", d.Target)
	}
	res.Target = d.Target
	if f, ok := readFailedIn(o.DataDir); ok && f.Target == d.Target && !o.Apply {
		res.Action = ActionHold
		res.Reason = fmt.Sprintf("%s failed earlier (%s); to try it again run: %s", d.Target, f.Reason, ForceCommand(d.Target))
		return res, nil
	}
	if o.Check {
		res.Action = ActionAvailable
		return res, nil
	}
	if !o.Apply && !d.AutoApply {
		res.Action = ActionAvailable
		res.Reason += " (not applied automatically; to install now run: " + ForceCommand(d.Target) + ")"
		return res, nil
	}
	detail, err := Install(ctx, o, d.Target)
	res.Detail = detail
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			// Not a bad release, so do not hold the target; tell the operator what to run.
			return res, fmt.Errorf("%w; next step: run as the user that owns %s, for example: sudo %s", err, o.ExePath, ForceCommand(d.Target))
		}
		if o.DataDir != "" && !errors.Is(err, ErrBusy) {
			_ = WriteFailed(o.DataDir, Failed{Target: d.Target, At: time.Now(), Reason: err.Error()})
		}
		return res, err
	}
	if o.DataDir != "" {
		_ = os.Remove(filepath.Join(o.DataDir, failedFile))
		if d.Action == nodeinsight.UpdateRollback {
			// Remember the version we left so it is not offered straight back.
			_ = WriteFailed(o.DataDir, Failed{Target: o.Current, At: time.Now(), Reason: "rolled back after failed health"})
		}
	}
	res.Previous = PrevPath(o.ExePath)
	res.Action = ActionStaged
	if o.Service == nil {
		return res, nil
	}
	rctx, cancel := context.WithTimeout(ctx, restartTimeout)
	defer cancel()
	if err := o.Service.Restart(rctx); err != nil {
		// The new binary is in place; it runs at the next manual restart.
		return res, fmt.Errorf("installed %s but could not restart the agent: %w", d.Target, err)
	}
	res.Action = ActionRestarted
	return res, nil
}

// Install downloads target, verifies checksum and signature (never skipped),
// keeps the current binary as .prev and swaps the new one in atomically.
func Install(ctx context.Context, o Options, target string) (string, error) {
	unlock, err := lock(o.ExePath + ".lock")
	if err != nil {
		return "", err
	}
	defer unlock()
	stage, f, err := stageFile(o.ExePath)
	if err != nil {
		return "", err
	}
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(stage)
		}
	}()
	work, err := os.MkdirTemp("", "agent-update-")
	if err != nil {
		_ = f.Close()
		return "", fmt.Errorf("create download directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(work) }()

	fetched, err := o.Release.Fetch(ctx, target, o.Asset, work)
	if err != nil {
		_ = f.Close()
		return "", err
	}
	detail, err := o.Release.Verify(ctx, fetched.ChecksumsPath, fetched.BundlePath)
	if err != nil {
		_ = f.Close()
		return "", err
	}
	if err := copyInto(f, fetched.BinaryPath); err != nil {
		return detail, err
	}
	// Hash the staged copy so the bytes that get renamed are the bytes checked.
	if err := selfupgrade.VerifyChecksum(stage, fetched.ChecksumsPath, o.Asset); err != nil {
		return detail, err
	}
	// The signature covers the checksums, not the tag, so the staged binary's
	// own version is the only tie between the requested tag and these bytes.
	if o.Smoke == nil {
		return detail, errors.New("refusing to install without a version check of the downloaded agent")
	}
	got, err := o.Smoke(ctx, stage)
	if err != nil {
		return detail, fmt.Errorf("the downloaded agent did not run: %w", err)
	}
	if got != target {
		return detail, fmt.Errorf("%w: wanted %s, got %s", selfupgrade.ErrVersionMismatch, target, got)
	}
	prev := PrevPath(o.ExePath)
	_ = os.Remove(prev)
	// A hard link keeps the running path valid at every instant; copy-free .prev.
	if err := os.Link(o.ExePath, prev); err != nil {
		if err := os.Rename(o.ExePath, prev); err != nil {
			return detail, fmt.Errorf("keep the current binary as %s: %w", prev, err)
		}
	}
	if err := replace(stage, o.ExePath, prev); err != nil {
		return detail, fmt.Errorf("replace %s: %w", o.ExePath, err)
	}
	renamed = true
	syncDir(filepath.Dir(o.ExePath))
	if o.DataDir != "" {
		if err := WriteMarker(o.DataDir, Marker{From: o.Current, To: target, InstalledAt: time.Now()}); err != nil {
			return detail, fmt.Errorf("record the install: %w", err)
		}
	}
	return detail, nil
}

// replace moves src over dst; when dst is missing (moved aside to prev on a
// platform that cannot link) it still lands, and a failure restores prev.
func replace(src, dst, prev string) error {
	if err := os.Rename(src, dst); err != nil {
		_ = os.Rename(prev, dst)
		return err
	}
	return nil
}

// Rollback puts the .prev binary back and restarts. The failed binary is kept
// as .failed, and the target is remembered so it is not retried in a loop.
func Rollback(ctx context.Context, exe, dataDir string, svc Service) (Result, error) {
	unlock, err := lock(exe + ".lock")
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	prev := PrevPath(exe)
	if _, err := os.Stat(prev); err != nil {
		return Result{Action: ActionNone}, fmt.Errorf("no previous binary to return to: %w", err)
	}
	failed := exe + ".failed"
	_ = os.Remove(failed)
	if err := os.Link(exe, failed); err != nil {
		if err := os.Rename(exe, failed); err != nil {
			return Result{}, fmt.Errorf("move the failing binary aside: %w", err)
		}
	}
	if err := os.Rename(prev, exe); err != nil {
		_ = os.Rename(failed, exe)
		return Result{}, fmt.Errorf("restore %s: %w", prev, err)
	}
	syncDir(filepath.Dir(exe))
	if dataDir != "" {
		if m, ok := ReadMarker(dataDir); ok {
			_ = WriteFailed(dataDir, Failed{Target: m.To, At: time.Now(), Reason: "rolled back after failed health"})
		}
		ClearMarker(dataDir)
	}
	res := Result{Action: ActionRolled, Previous: failed}
	if svc == nil {
		return res, nil
	}
	rctx, cancel := context.WithTimeout(ctx, restartTimeout)
	defer cancel()
	if err := svc.Restart(rctx); err != nil {
		return res, fmt.Errorf("restored the previous binary but could not restart: %w", err)
	}
	return res, nil
}

// stageFile creates the temp file beside the executable and keeps its open
// handle, so the bytes written are the file that is later renamed.
func stageFile(exe string) (string, *os.File, error) {
	mode := fs.FileMode(0o755)
	if st, err := os.Stat(exe); err == nil {
		mode = st.Mode().Perm() | 0o111
	}
	f, err := os.CreateTemp(filepath.Dir(exe), "."+filepath.Base(exe)+".update-*")
	if err != nil {
		return "", nil, fmt.Errorf("cannot write next to the agent executable (%s): %w", filepath.Dir(exe), err)
	}
	name := f.Name()
	if err := f.Chmod(mode); err != nil && runtime.GOOS != "windows" {
		_ = f.Close()
		_ = os.Remove(name)
		return "", nil, fmt.Errorf("set permissions on staged file: %w", err)
	}
	return name, f, nil
}

// copyInto fills the staged handle from src, then syncs and closes it.
func copyInto(out *os.File, src string) error {
	in, err := os.Open(src) //nolint:gosec // src is inside the update's own temp directory
	if err != nil {
		_ = out.Close()
		return fmt.Errorf("open download: %w", err)
	}
	defer func() { _ = in.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("write staged file: %w", err)
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return fmt.Errorf("sync staged file: %w", err)
	}
	return out.Close()
}

// SystemService restarts the agent through the host service manager, with a
// fixed argv and no shell.
type SystemService struct {
	Name string
	// Domain is the launchd domain target (gui/501 or system); empty derives
	// it from where the unit is installed.
	Domain string
	// GOOS is overridable for tests.
	GOOS string
	Run  func(ctx context.Context, argv []string) error
}

// Argv returns the restart command for the service manager of goos.
func (s SystemService) Argv() ([]string, error) {
	if !serviceNameRE.MatchString(s.Name) {
		return nil, fmt.Errorf("unsafe service name %q", s.Name)
	}
	goos := s.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	switch goos {
	case "linux":
		return []string{"systemctl", "restart", s.Name}, nil
	case "darwin":
		return []string{"launchctl", "kickstart", "-k", s.launchdDomain() + "/" + s.Name}, nil
	case "windows":
		return []string{"powershell", "-NoProfile", "-Command", "Restart-Service -Name '" + s.Name + "'"}, nil
	}
	return nil, ErrNoServiceManager
}

// launchdDomain is gui/<uid> when the unit is a per-user LaunchAgent, else system.
func (s SystemService) launchdDomain() string {
	if s.Domain != "" {
		return s.Domain
	}
	if home, err := os.UserHomeDir(); err == nil {
		if _, serr := os.Stat(filepath.Join(home, "Library", "LaunchAgents", s.Name+".plist")); serr == nil {
			return "gui/" + strconv.Itoa(os.Getuid())
		}
	}
	return "system"
}

// Restart runs the restart command.
func (s SystemService) Restart(ctx context.Context) error {
	argv, err := s.Argv()
	if err != nil {
		return err
	}
	if s.Run != nil {
		return s.Run(ctx, argv)
	}
	if out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput(); err != nil { //nolint:gosec // fixed argv from Argv, service name from operator env
		return fmt.Errorf("%s: %w: %s", argv[0], err, out)
	}
	return nil
}

// DefaultServiceName is the service label the packaged installers register.
func DefaultServiceName(goos string) string {
	if goos == "darwin" {
		return "com.levelrail.agent"
	}
	return "levelrail-agent"
}

func readFailedIn(dir string) (Failed, bool) {
	if dir == "" {
		return Failed{}, false
	}
	return ReadFailed(dir)
}
