package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/vmruntime"
	"github.com/GLINCKER/levelrail/internal/vmruntime/guest"
)

const (
	vmGuestRoot      = "/var/lib/levelrail"
	vmBackupPrefix   = "upgrade-backup-"
	vmBackupStamp    = "20060102T150405Z"
	vmExportDataName = "vm-data.tar"
	vmExportSettings = "settings.json"
	vmRollbackGrace  = 2 * time.Minute
	vmExchangeWait   = 15 * time.Second
	vmFileMode       = 0o600
)

// vmStreamer is an optional runner door that streams stdout without buffering it all.
type vmStreamer interface {
	Stream(ctx context.Context, w io.Writer, args ...string) error
}

func (e *vmEnv) stream(ctx context.Context, w io.Writer, args ...string) error {
	if s, ok := e.runner.(vmStreamer); ok {
		return s.Stream(ctx, w, args...)
	}
	out, err := e.runner.Run(ctx, args...)
	if err != nil {
		return err
	}
	_, err = w.Write(out)
	return err
}

func (e *vmEnv) requireRunning(ctx context.Context) error {
	inst, err := e.instance(ctx)
	if err != nil {
		return err
	}
	if inst == nil {
		return errors.New("no VM found; run `levelrail vm init`")
	}
	if inst.Status != "Running" {
		return errors.New("the VM is not running; run `levelrail vm start`")
	}
	return nil
}

func (e *vmEnv) guestSh(ctx context.Context, args ...string) error {
	_, err := e.runner.Run(ctx, append([]string{"shell", vmInstance, "--"}, args...)...)
	return err
}

// export writes the guest data dir (which holds the control plane database) plus the host
// settings into one 0600 archive while the control plane is stopped, so the copy is consistent.
func (e *vmEnv) export(ctx context.Context, out, recipient string, stdout io.Writer) (err error) {
	out, err = filepath.Abs(out)
	if err != nil {
		return err
	}
	if rel, rerr := filepath.Rel(e.limaDir(), out); rerr == nil && !strings.HasPrefix(rel, "..") {
		return errors.New("choose an export path outside the VM state directory")
	}
	var rec age.Recipient
	if recipient != "" {
		if rec, err = age.ParseX25519Recipient(strings.TrimSpace(recipient)); err != nil {
			return fmt.Errorf("--encrypt wants an age X25519 public key (age1...): %w", err)
		}
	}
	settings, err := os.ReadFile(e.settingsPath) //nolint:gosec // per-user settings file
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}
	if err = e.requireRunning(ctx); err != nil {
		return err
	}
	// O_EXCL refuses to overwrite and fixes the mode before any data lands.
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, vmFileMode) //nolint:gosec // user-chosen export path
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s already exists; export never overwrites", out)
		}
		return err
	}
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(out) //nolint:gosec // operator-supplied output path
		}
	}()
	tmp, err := os.CreateTemp(filepath.Dir(out), ".vm-export-*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }() //nolint:gosec // temp file we created
	if err = e.tarGuestData(ctx, tmp); err != nil {
		return err
	}
	size, err := tmp.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	if _, err = tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	sum := sha256.New()
	dst := io.Writer(io.MultiWriter(f, sum))
	var aw io.WriteCloser
	if rec != nil {
		if aw, err = age.Encrypt(dst, rec); err != nil {
			return err
		}
		dst = aw
	}
	tw := tar.NewWriter(dst)
	if err = putTar(tw, vmExportDataName, size, tmp); err != nil {
		return err
	}
	if err = putTar(tw, vmExportSettings, int64(len(settings)), bytes.NewReader(settings)); err != nil {
		return err
	}
	if err = tw.Close(); err != nil {
		return err
	}
	if aw != nil {
		if err = aw.Close(); err != nil {
			return err
		}
	}
	if err = f.Close(); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "Exported to %s\nsha256: %s\n", out, hex.EncodeToString(sum.Sum(nil)))
	if rec == nil {
		_, _ = fmt.Fprintln(stdout, "WARNING: the archive is NOT encrypted. It contains the control plane database and its credentials: keep it private (mode 0600) or re-run with --encrypt.")
	} else {
		_, _ = fmt.Fprintln(stdout, "The archive is encrypted to the given age key. Keep the matching private key safe; without it the archive cannot be read.")
	}
	return nil
}

func putTar(tw *tar.Writer, name string, size int64, r io.Reader) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: vmFileMode, Size: size, ModTime: time.Now().UTC()}); err != nil {
		return err
	}
	_, err := io.Copy(tw, r)
	return err
}

// tarGuestData streams the data dir with the control plane stopped, then restarts it.
func (e *vmEnv) tarGuestData(ctx context.Context, w io.Writer) error {
	active := e.guestSh(ctx, "sudo", "systemctl", "is-active", "--quiet", vmGuestUnit) == nil
	if active {
		if err := e.guestSh(ctx, "sudo", "systemctl", "stop", vmGuestUnit); err != nil {
			return fmt.Errorf("stop the control plane: %w", err)
		}
	}
	terr := e.stream(ctx, w, "shell", vmInstance, "--", "sudo", "tar", "-C", vmGuestRoot, "-cf", "-", "data")
	if active {
		if serr := e.guestSh(context.WithoutCancel(ctx), "sudo", "systemctl", "start", vmGuestUnit); serr != nil && terr == nil {
			terr = fmt.Errorf("the export is complete but the control plane did not restart: %w", serr)
		}
	}
	return terr
}

type vmDeletion struct {
	paths []string
	notes []string
}

func (e *vmEnv) deletionPlan(uninstall, keepCreds bool) vmDeletion {
	d := vmDeletion{paths: []string{e.instanceDir(), filepath.Join(e.dataDir, "levelrail.yaml"), e.settingsPath}}
	if uninstall {
		d.paths = append(d.paths, e.limaDir())
	}
	if uninstall || !keepCreds {
		d.paths = append(d.paths, e.adminPath())
	} else {
		d.notes = append(d.notes, "kept: "+e.adminPath()+" (first-admin credentials)")
	}
	d.notes = append(d.notes, "kept: any archive made by `vm export`")
	return d
}

func (e *vmEnv) printDeletion(w io.Writer, uninstall, keepCreds bool) {
	d := e.deletionPlan(uninstall, keepCreds)
	_, _ = fmt.Fprintln(w, "This will delete the VM named "+vmInstance+" with its disk, and these host paths:")
	for _, p := range d.paths {
		_, _ = fmt.Fprintln(w, "  delete: "+p)
	}
	for _, n := range d.notes {
		_, _ = fmt.Fprintln(w, "  "+n)
	}
}

// reset deletes the VM and its host state; uninstall also removes the Lima home and credentials.
func (e *vmEnv) reset(ctx context.Context, uninstall, keepCreds bool, stdout io.Writer) error {
	e.printDeletion(stdout, uninstall, keepCreds)
	inst, err := e.instance(ctx)
	if err != nil {
		return err
	}
	if inst != nil {
		if inst.Status == "Running" {
			if _, err := e.runner.Run(ctx, "stop", "--force", vmInstance); err != nil {
				return err
			}
		}
		if _, err := e.runner.Run(ctx, "delete", "--force", vmInstance); err != nil {
			return err
		}
	}
	for _, p := range e.deletionPlan(uninstall, keepCreds).paths {
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	if uninstall {
		// Only an empty data dir goes; a pinned limactl or an export in it stays.
		_ = os.Remove(e.dataDir)
	}
	_, _ = fmt.Fprintln(stdout, "Done.")
	return nil
}

// guestChange is one guest file the upgrade rewrites.
type guestChange struct {
	path, mode string
	content    string
}

// reprovision rewrites the unit and egress files that differ and reports what changed.
func (e *vmEnv) reprovision(ctx context.Context, s vmruntime.Settings) (egress, units bool, err error) {
	ls := s.ToLima()
	files, err := guest.Files(ls.Guest, vmSidecar(s))
	if err != nil {
		return false, false, err
	}
	keep := map[string]bool{
		guest.EgressRulesPath: true, guest.CheckScriptPath: true,
		"/etc/systemd/system/" + guest.IsolationUnit: true, "/etc/systemd/system/" + guest.CheckUnit: true,
		"/etc/systemd/system/" + vmGuestUnit: true,
	}
	tmp, err := os.MkdirTemp("", "lr-upgrade-*")
	if err != nil {
		return false, false, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	for i, f := range files {
		if !keep[f.Path] {
			continue
		}
		want := f.Content
		if !strings.HasSuffix(want, "\n") {
			want += "\n"
		}
		if cur, rerr := e.guestRead(ctx, f.Path); rerr == nil && string(cur) == want {
			continue
		}
		src := filepath.Join(tmp, fmt.Sprintf("f%d", i))
		if err := os.WriteFile(src, []byte(want), vmFileMode); err != nil {
			return false, false, err
		}
		guestTmp := fmt.Sprintf("/tmp/lr-upgrade-%d", i)
		for _, a := range [][]string{
			{"copy", src, vmInstance + ":" + guestTmp},
			{"shell", vmInstance, "--", "sudo", "install", "-D", "-m", f.Mode, guestTmp, f.Path},
			{"shell", vmInstance, "--", "rm", "-f", guestTmp},
		} {
			if _, err := e.runner.Run(ctx, a...); err != nil {
				return false, false, err
			}
		}
		if f.Path == guest.EgressRulesPath {
			egress = true
		}
		units = true
	}
	return egress, units, nil
}

// upgrade swaps the guest control plane behind a health gate and rolls back on failure.
func (e *vmEnv) upgrade(ctx context.Context, bin string, stdout io.Writer, lookupEnv vmruntime.Lookup) error {
	if bin == "" {
		return errors.New("--control-plane-bin PATH is required (a linux build)")
	}
	if fi, err := os.Stat(bin); err != nil || fi.IsDir() {
		return fmt.Errorf("control plane binary %s is not readable", bin)
	}
	if err := e.checkLimaHome(); err != nil {
		return err
	}
	s, err := e.load()
	if err != nil {
		return err
	}
	timeout, err := s.UpgradeTimeout(lookupEnv)
	if err != nil {
		return err
	}
	if err := e.requireRunning(ctx); err != nil {
		return err
	}
	l, err := e.limits(lookupEnv)
	if err != nil {
		return err
	}
	yaml, err := e.render(s, l)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(e.instanceDir(), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(e.instanceDir(), "lima.yaml"), yaml, vmFileMode); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stdout, "Updating the guest unit and egress rules...")
	egress, units, err := e.reprovision(ctx, s)
	if err != nil {
		return err
	}
	if units {
		if err := e.guestSh(ctx, "sudo", "systemctl", "daemon-reload"); err != nil {
			return err
		}
	}
	if egress {
		if err := e.guestSh(ctx, "sudo", "systemctl", "restart", guest.IsolationUnit); err != nil {
			return err
		}
	}
	stamp := time.Now().UTC().Format(vmBackupStamp)
	backup := vmGuestRoot + "/" + vmBackupPrefix + stamp
	hadBin := e.guestSh(ctx, "test", "-x", vmGuestBin) == nil
	_, _ = fmt.Fprintf(stdout, "Stopping the control plane and backing up to %s ...\n", backup)
	if err := e.guestSh(ctx, "sudo", "systemctl", "stop", vmGuestUnit); err != nil {
		return err
	}
	steps := [][]string{{"sudo", "install", "-d", "-m", "0700", backup}, {"sudo", "cp", "-a", vmGuestData, backup + "/data"}}
	if hadBin {
		steps = append(steps, []string{"sudo", "cp", "-a", vmGuestBin, backup + "/levelrail.bin"})
	}
	for _, a := range steps {
		if err := e.guestSh(ctx, a...); err != nil {
			_ = e.guestSh(context.WithoutCancel(ctx), "sudo", "systemctl", "start", vmGuestUnit)
			return fmt.Errorf("backup failed, nothing changed: %w", err)
		}
	}
	rec := &vmruntime.UpgradeBackup{Path: backup, At: time.Now().UTC().Format(time.RFC3339)}
	gateErr := e.swapAndGate(ctx, s, bin, timeout, stdout)
	if gateErr == nil {
		rec.Result = "ok"
		s.UpgradeBackup = rec
		_ = e.guestSh(ctx, "sudo", "find", vmGuestRoot, "-maxdepth", "1", "-name", vmBackupPrefix+"*", "!", "-name", vmBackupPrefix+stamp, "-exec", "rm", "-rf", "{}", "+")
		if err := vmruntime.Save(e.settingsPath, s); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "Upgraded. Backup kept at %s in the guest.\n", backup)
		return nil
	}
	rec.Result = "rolled_back"
	s.UpgradeBackup = rec
	rbErr := e.rollback(context.WithoutCancel(ctx), s, backup, hadBin, stdout)
	_ = vmruntime.Save(e.settingsPath, s)
	if rbErr != nil {
		rec.Result = "rollback_failed"
		_ = vmruntime.Save(e.settingsPath, s)
		return fmt.Errorf("upgrade failed (%w) and the rollback also failed (%w); backup is at %s", gateErr, rbErr, backup)
	}
	if errors.Is(gateErr, errIsolation) {
		// An unsafe guest must not keep running, even on the old version.
		e.failClosed(ctx)
		return fmt.Errorf("upgrade failed and was rolled back, and the VM was stopped: %w", gateErr)
	}
	return fmt.Errorf("upgrade failed and was rolled back: %w", gateErr)
}

var errIsolation = errors.New("isolation check failed after the upgrade")

func (e *vmEnv) swapAndGate(ctx context.Context, s vmruntime.Settings, bin string, timeout time.Duration, stdout io.Writer) error {
	deadline := time.Now().Add(timeout)
	gctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	for _, a := range [][]string{
		{"copy", bin, vmInstance + ":/tmp/levelrail-cp"},
		{"shell", vmInstance, "--", "sudo", "install", "-m", "0755", "/tmp/levelrail-cp", vmGuestBin},
		{"shell", vmInstance, "--", "sudo", "systemctl", "start", vmGuestUnit},
	} {
		if _, err := e.runner.Run(gctx, a...); err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintf(stdout, "Waiting up to %s for the control plane...\n", timeout)
	if err := e.waitDashboard(gctx, s.DashboardPort, deadline); err != nil {
		return err
	}
	// A fresh self-check, so a pass from before the upgrade never counts.
	if err := e.guestSh(gctx, "sudo", "systemctl", "restart", guest.CheckUnit); err != nil {
		return err
	}
	if err := e.waitIsolation(gctx, deadline); err != nil {
		return fmt.Errorf("%w: %w", errIsolation, err)
	}
	return nil
}

func (e *vmEnv) rollback(ctx context.Context, s vmruntime.Settings, backup string, hadBin bool, stdout io.Writer) error {
	_, _ = fmt.Fprintln(stdout, "Rolling back to the previous version...")
	steps := [][]string{{"sudo", "systemctl", "stop", vmGuestUnit}}
	if hadBin {
		steps = append(steps, []string{"sudo", "install", "-m", "0755", backup + "/levelrail.bin", vmGuestBin})
	}
	steps = append(steps,
		[]string{"sudo", "rm", "-rf", vmGuestData},
		[]string{"sudo", "cp", "-a", backup + "/data", vmGuestData},
		[]string{"sudo", "systemctl", "start", vmGuestUnit},
	)
	for _, a := range steps {
		if err := e.guestSh(ctx, a...); err != nil {
			return err
		}
	}
	deadline := time.Now().Add(vmRollbackGrace)
	rctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	return e.waitDashboard(rctx, s.DashboardPort, deadline)
}

// token serves the tray app: --check tests the guest token file, --exchange trades it for a
// session cookie. The token itself is never printed.
func (e *vmEnv) token(ctx context.Context, f vmFlags, stdout io.Writer) error {
	switch {
	case f.check == f.exchange:
		return errors.New("choose exactly one of --check or --exchange; the token is never printed")
	case f.check:
		if !vmruntime.TokenExists(ctx, e.runner) {
			return errors.New("the desktop token file does not exist yet")
		}
		return nil
	}
	if f.cookieJar == "" {
		return errors.New("--exchange needs --cookie-jar PATH")
	}
	s, err := e.load()
	if err != nil {
		return err
	}
	tok, err := vmruntime.ReadToken(ctx, e.runner)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: vmExchangeWait, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	cookie, exp, err := vmruntime.Exchange(ctx, client, s.DashboardPort, tok)
	if err != nil {
		return err
	}
	if err := writeCookieJar(f.cookieJar, cookie); err != nil {
		return err
	}
	if f.jsonOut {
		_, _ = fmt.Fprintf(stdout, "{\"expires_at\":%q}\n", exp)
		return nil
	}
	_, _ = fmt.Fprintf(stdout, "expires_at: %s\n", exp)
	return nil
}

// writeCookieJar writes a curl-style cookie file, mode 0600, atomically.
func writeCookieJar(path string, c *http.Cookie) error {
	if strings.ContainsAny(c.Value, "\t\r\n") {
		return errors.New("unexpected cookie value")
	}
	exp := int64(0)
	if !c.Expires.IsZero() {
		exp = c.Expires.Unix()
	}
	line := fmt.Sprintf("#HttpOnly_%s\tFALSE\t%s\t%s\t%d\t%s\t%s\n", vmruntime.DashboardHost, c.Path, "FALSE", exp, c.Name, c.Value)
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cookie-*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(vmFileMode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString("# Netscape HTTP Cookie File\n" + line); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
