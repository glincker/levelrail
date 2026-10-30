package mailcert

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"time"
)

// Volume is the mail config volume as the installer needs it.
type Volume interface {
	// Read returns the file, or found false when it does not exist.
	Read(ctx context.Context, path string) (data []byte, found bool, err error)
	Write(ctx context.Context, path string, data []byte, private bool) error
	Mode(ctx context.Context, path string) (fs.FileMode, error)
	Remove(ctx context.Context, path string) error
}

// Switcher points mox at the managed files and restarts it, or undoes that.
type Switcher interface {
	Activate(ctx context.Context) error
	Revert(ctx context.Context) error
	Served(ctx context.Context) (Info, error)
}

// Outcome values for Result.Outcome.
const (
	OutcomeInstalled  = "installed"
	OutcomeRolledBack = "rolled_back"
	OutcomeFailed     = "failed"
)

// Result reports an install. Reason is a short code, never key material.
type Result struct {
	Outcome string
	Reason  string
	Served  Info
	HadPrev bool
}

// Install writes the pair into the volume, keeps the previous managed pair as
// rollback, restarts mox and waits until it serves want. Any failure after the
// files change restores the previous pair and restarts again.
func Install(ctx context.Context, cfg Config, vol Volume, sw Switcher, certPEM, keyPEM []byte, want Info) (Result, error) {
	prevCert, hadCert, err := vol.Read(ctx, CertPath)
	if err != nil {
		return Result{Outcome: OutcomeFailed, Reason: "read_current"}, fmt.Errorf("mail cert: read current certificate: %w", err)
	}
	prevKey, hadKey, err := vol.Read(ctx, KeyPath)
	if err != nil {
		return Result{Outcome: OutcomeFailed, Reason: "read_current"}, fmt.Errorf("mail cert: read current key: %w", err)
	}
	hadPrev := hadCert && hadKey
	res := Result{HadPrev: hadPrev}
	if hadPrev {
		if err := writePair(ctx, vol, PrevCertPath, PrevKeyPath, prevCert, prevKey); err != nil {
			res.Outcome, res.Reason = OutcomeFailed, "save_previous"
			return res, err
		}
	}
	if err := writePair(ctx, vol, CertPath, KeyPath, certPEM, keyPEM); err != nil {
		res.Outcome, res.Reason = OutcomeFailed, "write"
		restore(ctx, vol, hadPrev, prevCert, prevKey)
		return res, err
	}
	if err := sw.Activate(ctx); err != nil {
		return rollback(ctx, vol, sw, res, prevCert, prevKey, "restart", err)
	}
	served, err := waitServed(ctx, cfg, sw, want.Serial)
	if err != nil {
		return rollback(ctx, vol, sw, res, prevCert, prevKey, "verify", err)
	}
	res.Outcome, res.Served = OutcomeInstalled, served
	return res, nil
}

func writePair(ctx context.Context, vol Volume, certPath, keyPath string, certPEM, keyPEM []byte) error {
	if err := vol.Write(ctx, keyPath, keyPEM, true); err != nil {
		return fmt.Errorf("mail cert: write key: %w", err)
	}
	mode, err := vol.Mode(ctx, keyPath)
	if err != nil || mode.Perm()&0o077 != 0 {
		_ = vol.Remove(ctx, keyPath)
		if err != nil {
			return fmt.Errorf("mail cert: check key mode: %w", err)
		}
		return ErrKeyModeTooWide
	}
	if err := vol.Write(ctx, certPath, certPEM, false); err != nil {
		return fmt.Errorf("mail cert: write certificate: %w", err)
	}
	return nil
}

func restore(ctx context.Context, vol Volume, hadPrev bool, prevCert, prevKey []byte) {
	if hadPrev {
		_ = writePair(ctx, vol, CertPath, KeyPath, prevCert, prevKey)
		return
	}
	_ = vol.Remove(ctx, KeyPath)
	_ = vol.Remove(ctx, CertPath)
}

func rollback(ctx context.Context, vol Volume, sw Switcher, res Result, prevCert, prevKey []byte, reason string, cause error) (Result, error) {
	restore(ctx, vol, res.HadPrev, prevCert, prevKey)
	res.Outcome, res.Reason = OutcomeRolledBack, reason
	if err := sw.Revert(ctx); err != nil {
		res.Outcome = OutcomeFailed
		return res, errors.Join(cause, fmt.Errorf("mail cert: rollback restart: %w", err))
	}
	return res, cause
}

func waitServed(ctx context.Context, cfg Config, sw Switcher, serial string) (Info, error) {
	deadline := time.Now().Add(cfg.VerifyTimeout)
	var last error
	for {
		got, err := sw.Served(ctx)
		if err == nil && got.Serial == serial {
			return got, nil
		}
		if err == nil {
			last = errors.New("mail cert: mox still serves the previous certificate")
		} else {
			last = err
		}
		if time.Now().Add(cfg.PollInterval).After(deadline) {
			return Info{}, last
		}
		select {
		case <-ctx.Done():
			return Info{}, ctx.Err()
		case <-time.After(cfg.PollInterval):
		}
	}
}
