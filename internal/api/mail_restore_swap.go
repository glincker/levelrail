package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/mail/mox"
	"github.com/GLINCKER/levelrail/internal/mailbackup/moxrun"
	"github.com/GLINCKER/levelrail/internal/store"
)

// mailRestoreHoldPrefix marks a maintenance hold owned by a mail restore.
const mailRestoreHoldPrefix = "mail-restore:"

const (
	mailRollbackDone   = "rolled_back"
	mailRollbackFailed = "rollback_failed"
)

// mailHolder persists the maintenance hold the reconciler honors.
type mailHolder interface {
	UpdateServiceMaintenanceHold(ctx context.Context, name, hold string) error
}

// mailVolumes finds the data and config volumes of the mail app.
func mailVolumes(svc store.DesiredService) moxrun.Volumes {
	var v moxrun.Volumes
	for _, m := range svc.Volumes {
		switch m.ContainerPath {
		case moxrun.DataDir:
			v.Data = m.Name
		case moxrun.ConfigDir:
			v.Config = m.Name
		}
	}
	return v
}

// mailStopped re-reads the container and fails unless it exists and is stopped.
func mailStopped(ctx context.Context, rt docker.Runtime, name string) error {
	st, err := rt.InspectByName(ctx, name)
	if err != nil || st == nil {
		return fmt.Errorf("the mail container could not be inspected: %w", err)
	}
	if st.Running {
		return errors.New("the mail container was started during the restore")
	}
	return nil
}

// mailSwap is one held, journaled swap of staged restore data into the live
// volumes, with rollback on any failure.
type mailSwap struct {
	rt     docker.Runtime
	holder mailHolder
	t      mailBackupTarget
	vols   moxrun.Volumes
	roots  []moxrun.Root
	step   func(string)
	logger *slog.Logger
	// ready overrides the admin API wait, for tests.
	ready func(ctx context.Context) error
}

func (s *mailSwap) release(ctx context.Context) error {
	return s.holder.UpdateServiceMaintenanceHold(context.WithoutCancel(ctx), s.t.svc.Name, "")
}

func (s *mailSwap) waitUp(ctx context.Context) error {
	if s.ready != nil {
		return s.ready(ctx)
	}
	wait := envMinutes("APP_MAIL_RESTORE_WAIT_MINUTES", 3)
	return moxrun.WaitRunning(ctx, s.rt, s.t.container.Name, wait, func(c docker.ContainerState) bool {
		ip := containerIP(c)
		return ip != "" && mox.AdminReachable(ctx, mox.AdminBaseURL(ip))
	})
}

// run holds the app, stops mail, swaps every root and verifies mail comes up.
// It returns the rollback outcome ("" when none ran) with the error.
func (s *mailSwap) run(ctx context.Context, id string) (string, error) {
	app, cid := s.t.svc.Name, s.t.container.ID
	s.step("hold")
	if err := s.holder.UpdateServiceMaintenanceHold(ctx, app, mailRestoreHoldPrefix+id); err != nil {
		return "", fmt.Errorf("hold the mail app before stopping it: %w", err)
	}
	s.step("stop")
	if err := s.rt.Stop(ctx, cid, 30*time.Second); err != nil {
		_ = s.rt.Start(context.WithoutCancel(ctx), cid)
		return "", errors.Join(fmt.Errorf("stop mail: %w", err), s.release(ctx))
	}
	if err := mailStopped(ctx, s.rt, s.t.container.Name); err != nil {
		return "", errors.Join(err, s.release(ctx))
	}
	h, err := moxrun.StartHelper(ctx, s.rt, s.t.svc.Image, app, s.vols, envMinutes("APP_MAIL_RESTORE_TIMEOUT_MINUTES", 60))
	if err != nil {
		_ = s.rt.Start(context.WithoutCancel(ctx), cid)
		return "", errors.Join(err, s.release(ctx))
	}
	defer h.Close()
	pre := moxrun.PreRestoreName(id, time.Now())
	s.step("swap")
	for _, r := range s.roots {
		if err := mailStopped(ctx, s.rt, s.t.container.Name); err != nil {
			return s.rollback(ctx, h, err)
		}
		if _, err := h.Run(ctx, "swap", r.Dir, id, pre, r.Sub); err != nil {
			return s.rollback(ctx, h, err)
		}
		if err := mailStopped(ctx, s.rt, s.t.container.Name); err != nil {
			return s.rollback(ctx, h, err)
		}
	}
	s.step("start")
	if err := s.rt.Start(ctx, cid); err != nil {
		return s.rollback(ctx, h, err)
	}
	s.step("wait")
	if err := s.waitUp(ctx); err != nil {
		return s.rollback(ctx, h, fmt.Errorf("the restored mail server did not come up: %w", err))
	}
	s.step("commit")
	if err := s.commit(ctx, h); err != nil {
		// Mail runs on the restored data; keep the hold so recovery finishes the commit.
		return "", fmt.Errorf("restore is live but its journal could not be closed, the next mail call finishes it: %w", err)
	}
	return "", s.release(ctx)
}

// commit closes the data journal first: it is the commit point recovery reads.
func (s *mailSwap) commit(ctx context.Context, h *moxrun.Helper) error {
	keep := strconv.Itoa(envInt("APP_MAIL_RESTORE_KEEP_PRE_RESTORE", 2))
	for _, r := range s.roots {
		if _, err := h.Run(ctx, "commit", r.Dir, "", "", keep); err != nil {
			return err
		}
	}
	return nil
}

// rollback stops mail, restores every root from its pre-restore copy and
// starts mail again. The hold is released only when that fully worked.
func (s *mailSwap) rollback(ctx context.Context, h *moxrun.Helper, cause error) (string, error) {
	ctx = context.WithoutCancel(ctx)
	s.step("rollback")
	_ = s.rt.Stop(ctx, s.t.container.ID, 30*time.Second)
	var errs []error
	if err := mailStopped(ctx, s.rt, s.t.container.Name); err != nil {
		errs = append(errs, err)
	} else {
		for i := len(s.roots) - 1; i >= 0; i-- {
			if _, err := h.Run(ctx, "rollback", s.roots[i].Dir); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) == 0 {
		if err := s.rt.Start(ctx, s.t.container.ID); err != nil {
			errs = append(errs, err)
		} else if err := s.waitUp(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		s.logger.Error("api: mail restore rollback failed, the app stays held", slog.String("app", s.t.svc.Name), slog.String("error", errors.Join(errs...).Error()))
		return mailRollbackFailed, fmt.Errorf("restore failed (%w) and the rollback failed too (%w); the mail app stays held and the previous data is kept under %s on its volumes", cause, errors.Join(errs...), moxrun.WorkDir)
	}
	if err := s.release(ctx); err != nil {
		return mailRollbackDone, fmt.Errorf("restore failed and was rolled back (%w), but the hold could not be released: %w", cause, err)
	}
	return mailRollbackDone, fmt.Errorf("restore failed and was rolled back to the previous data: %w", cause)
}

// recover finishes an interrupted swap found by its persisted hold: a closed
// data journal means the restore committed, anything else is rolled back.
func (s *mailSwap) recover(ctx context.Context) (string, error) {
	h, err := moxrun.StartHelper(ctx, s.rt, s.t.svc.Image, s.t.svc.Name, s.vols, 10*time.Minute)
	if err != nil {
		return "", err
	}
	defer h.Close()
	state, err := h.Run(ctx, "state", moxrun.DataDir)
	if err != nil {
		return "", err
	}
	if state == "none" {
		if err := s.commit(ctx, h); err != nil {
			return "", err
		}
		if st, err := s.rt.InspectByName(ctx, s.t.container.Name); err == nil && st != nil && !st.Running {
			_ = s.rt.Start(ctx, s.t.container.ID)
		}
		return "", s.release(ctx)
	}
	return s.rollback(ctx, h, errors.New("a restore was interrupted before it finished"))
}

// rootsFor lists the volumes to swap: data always, config when staged.
func rootsFor(vols moxrun.Volumes, withConfig bool) []moxrun.Root {
	roots := []moxrun.Root{{Dir: moxrun.DataDir, Sub: "data"}}
	if withConfig && vols.Config != "" {
		roots = append(roots, moxrun.Root{Dir: moxrun.ConfigDir, Sub: "config"})
	}
	return roots
}

// sweepHostLeftovers removes plaintext staging and restore dirs on the
// control plane older than APP_MAIL_RESTORE_SWEEP_MINUTES, skipping busy apps.
func (c *mailBackupConfig) sweepHostLeftovers(now time.Time) {
	age := envMinutes("APP_MAIL_RESTORE_SWEEP_MINUTES", 120)
	for _, base := range []string{"mail-backup-staging", "mail-restore"} {
		apps, _ := os.ReadDir(filepath.Join(c.dataDir, base))
		for _, a := range apps {
			if j := c.current(a.Name()); j != nil && j.State == mailJobRunning {
				continue
			}
			dir := filepath.Join(c.dataDir, base, a.Name())
			ents, _ := os.ReadDir(dir)
			for _, e := range ents {
				if fi, err := e.Info(); err == nil && now.Sub(fi.ModTime()) > age {
					_ = os.RemoveAll(filepath.Join(dir, e.Name()))
				}
			}
		}
	}
}

// sweepVolume removes stale staged, discarded and legacy restore dirs on the
// mail volume through the running mail container.
func sweepVolume(ctx context.Context, rt moxrun.Execer, cid string, withConfig bool) {
	mins := strconv.Itoa(int(envMinutes("APP_MAIL_RESTORE_SWEEP_MINUTES", 120).Minutes()))
	roots := []string{moxrun.DataDir}
	if withConfig {
		roots = append(roots, moxrun.ConfigDir)
	}
	for _, r := range roots {
		_ = moxrun.Sweep(ctx, rt, cid, r, mins)
	}
}
