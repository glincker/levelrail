package cliupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
)

// Actions reported in Result.Action.
const (
	ActionUpToDate   = "up-to-date"
	ActionWouldApply = "would-update"
	ActionUpdated    = "updated"
)

const smokeTimeout = 10 * time.Second

// ErrNotWritable means the executable's directory cannot take a staged file.
var ErrNotWritable = errors.New("cannot write next to the running executable")

// Options configures one update run.
type Options struct {
	Source Source
	// Current is the running version, Target an explicit tag ("" means latest).
	Current string
	Target  string
	DryRun  bool
	// ExePath is the resolved path of the running executable.
	ExePath string
	// TUFCacheDir keeps Sigstore trust metadata between runs.
	TUFCacheDir string
	// Confirm runs after the plan is known and before any download; an error aborts.
	Confirm func(current, target string) error
	// Verify checks the signature over checksums.txt. Nil verifies in process,
	// in require mode, so a release without a valid signature is never applied.
	Verify func(ctx context.Context, checksumsPath, bundlePath string) (string, error)
}

// Result describes what a run did or would do.
type Result struct {
	Action       string `json:"action"`
	Current      string `json:"current"`
	Target       string `json:"target"`
	Asset        string `json:"asset"`
	Path         string `json:"path"`
	Verification string `json:"verification,omitempty"`
}

// ExecutablePath returns the running binary with symlinks resolved, so the
// swap replaces the real file and never a link.
func ExecutablePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate running executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", fmt.Errorf("resolve running executable: %w", err)
	}
	return resolved, nil
}

// Update replaces o.ExePath with the target release after verifying it. On
// any error before the final rename the current binary is untouched.
func Update(ctx context.Context, o Options) (Result, error) {
	res := Result{Current: o.Current, Asset: o.Source.Asset, Path: o.ExePath}
	if runtime.GOOS == "windows" {
		return res, errors.New("in-place update is not supported on Windows, re-run install-cli.ps1")
	}
	target := o.Target
	if target == "" {
		latest, err := o.Source.Latest(ctx)
		if err != nil {
			return res, err
		}
		if latest == "" {
			return res, fmt.Errorf("no release ships %s", o.Source.Asset)
		}
		if o.Current != "dev" && !Newer(o.Current, latest) {
			res.Target, res.Action = o.Current, ActionUpToDate
			return res, nil
		}
		if o.Current == "dev" {
			return res, errors.New("this is a development build, so there is nothing to compare against: pass --version <tag> to choose a release")
		}
		target = latest
	} else if !selfupgrade.ValidTag(target) {
		return res, fmt.Errorf("refusing unsafe release tag %q", target)
	}
	res.Target = target
	if target == o.Current {
		res.Action = ActionUpToDate
		return res, nil
	}
	if o.DryRun {
		res.Action = ActionWouldApply
		return res, nil
	}
	if o.Confirm != nil {
		if err := o.Confirm(o.Current, target); err != nil {
			return res, err
		}
	}
	detail, err := apply(ctx, o, target)
	res.Verification = detail
	if err != nil {
		return res, err
	}
	res.Action = ActionUpdated
	return res, nil
}

func apply(ctx context.Context, o Options, target string) (string, error) {
	stage, err := stageFile(o.ExePath)
	if err != nil {
		return "", err
	}
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(stage)
		}
	}()

	work, err := os.MkdirTemp("", "cli-update-")
	if err != nil {
		return "", fmt.Errorf("create download directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(work) }()

	fetched, err := selfupgrade.Fetcher{BaseURL: o.Source.DownloadBase, Client: o.Source.Client, AllowHTTP: o.Source.AllowHTTP}.
		Fetch(ctx, target, o.Source.Asset, work)
	if err != nil {
		return "", err
	}
	verify := o.Verify
	if verify == nil {
		verify = selfupgrade.Signer{
			Mode:           selfupgrade.VerifyRequire,
			IdentityRegexp: selfupgrade.ReleaseIdentityRegexp(o.Source.Repo),
			Issuer:         selfupgrade.CosignOIDCIssuer,
			TUFCacheDir:    o.TUFCacheDir,
		}.Verify
	}
	detail, err := verify(ctx, fetched.ChecksumsPath, fetched.BundlePath)
	if err != nil {
		return "", err
	}
	if err := copyInto(stage, fetched.BinaryPath); err != nil {
		return detail, err
	}
	// Hash the staged copy, not the download, so the bytes that get renamed are the bytes checked.
	if err := selfupgrade.VerifyChecksum(stage, fetched.ChecksumsPath, o.Source.Asset); err != nil {
		return detail, err
	}
	if err := smoke(ctx, stage, target); err != nil {
		return detail, err
	}
	if err := os.Rename(stage, o.ExePath); err != nil {
		return detail, fmt.Errorf("replace %s: %w", o.ExePath, err)
	}
	renamed = true
	return detail, nil
}

// stageFile creates the temp file beside the executable, which doubles as the
// writability probe and keeps the final rename on one filesystem.
func stageFile(exe string) (string, error) {
	mode := fs.FileMode(0o755)
	if st, err := os.Stat(exe); err == nil {
		mode = st.Mode().Perm() | 0o111
	}
	f, err := os.CreateTemp(filepath.Dir(exe), "."+filepath.Base(exe)+".update-*")
	if err != nil {
		return "", fmt.Errorf("%w (%s): %w. Install the CLI somewhere you own, or re-run with sudo, or update it through the package manager that installed it", ErrNotWritable, filepath.Dir(exe), err)
	}
	name := f.Name()
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", fmt.Errorf("set permissions on staged file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("close staged file: %w", err)
	}
	return name, nil
}

func copyInto(dst, src string) error {
	in, err := os.Open(src) //nolint:gosec // src is inside the update's own temp directory
	if err != nil {
		return fmt.Errorf("open download: %w", err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_TRUNC, 0) //nolint:gosec // dst was just created by stageFile
	if err != nil {
		return fmt.Errorf("open staged file: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("write staged file: %w", err)
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return fmt.Errorf("sync staged file: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close staged file: %w", err)
	}
	return nil
}

// smoke runs the verified binary once to confirm it starts and reports the
// requested version. Fixed argv, no shell, a timeout and a scrubbed environment.
func smoke(ctx context.Context, path, want string) error {
	ctx, cancel := context.WithTimeout(ctx, smokeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "version", "--json") //nolint:gosec // path is the staged, checksum-verified binary
	cmd.Env = []string{"LEVELRAIL_CI=1"}
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("the downloaded binary did not run: %w", err)
	}
	var report struct {
		CLI struct {
			Version string `json:"version"`
		} `json:"cli"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		return fmt.Errorf("the downloaded binary printed an unreadable version: %w", err)
	}
	if report.CLI.Version != want {
		return fmt.Errorf("%w: wanted %s, got %s", selfupgrade.ErrVersionMismatch, want, report.CLI.Version)
	}
	return nil
}
