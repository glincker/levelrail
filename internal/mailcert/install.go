package mailcert

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"time"
)

// Volume is the mail config volume as the installer needs it. Remove of a
// missing file is not an error.
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
	OutcomeInstalled      = "installed"
	OutcomeRolledBack     = "rolled_back"
	OutcomeRollbackFailed = "rollback_failed"
	OutcomeFailed         = "failed"
)

// Journal phases: while staging the live pair is untouched, from swapping on it may hold the new pair.
const (
	PhaseStaging  = "staging"
	PhaseSwapping = "swapping"
)

// ErrInstallInProgress means a journal from another install is still inside its lease.
var ErrInstallInProgress = errors.New("mail cert: another certificate install is in progress, retry when it finishes")

// ErrKeyRemoveFailed means a key landed wider than 0600 and could not be deleted again.
var ErrKeyRemoveFailed = errors.New("mail cert: the node wrote the key wider than 0600 and deleting it failed, so the key may be readable by other users of the node: delete it by hand and upgrade the node agent")

// ErrInterrupted is the cause recorded when an earlier install did not finish.
var ErrInterrupted = errors.New("mail cert: an earlier install did not finish")

// Origin is the certificate setting the mail app had before an install, so a rollback can restore it.
type Origin struct {
	CertFile string `json:"cert_file,omitempty"`
	KeyFile  string `json:"key_file,omitempty"`
	Set      bool   `json:"set"`
}

// Journal is the persisted in-progress marker of one install. It holds no key material.
type Journal struct {
	Phase     string    `json:"phase"`
	StartedAt time.Time `json:"started_at"`
	NewSHA256 string    `json:"new_sha256"`
	HadPrev   bool      `json:"had_prev"`
	Origin    Origin    `json:"origin"`
}

// Stale reports whether the install that wrote j is past its lease and so cannot still be running.
func (j Journal) Stale(cfg Config, now time.Time) bool {
	return now.Sub(j.StartedAt) > cfg.InstallLease
}

// Result reports an install. Reason is a short code, never key material.
type Result struct {
	Outcome string
	Reason  string
	Served  Info
	HadPrev bool
}

// ReadJournal returns the journal of an unfinished install, if any.
func ReadJournal(ctx context.Context, vol Volume) (Journal, bool, error) {
	b, found, err := vol.Read(ctx, JournalPath)
	if err != nil || !found {
		return Journal{}, false, err
	}
	var j Journal
	if err := json.Unmarshal(b, &j); err != nil {
		// An unreadable journal still means an install did not finish.
		return Journal{Phase: PhaseSwapping, HadPrev: true}, true, nil
	}
	return j, true, nil
}

func writeJournal(ctx context.Context, vol Volume, j Journal) error {
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	if err := vol.Write(ctx, JournalPath, b, false); err != nil {
		return fmt.Errorf("mail cert: write install journal: %w", err)
	}
	return nil
}

// Install stages the pair under temporary names and checks it, keeps the
// previous managed pair as rollback, swaps the live pair, restarts mox and
// waits until it serves want. A journal marks the install until it ends, so
// an interrupted one is found by Recover. Rollback uses its own bounded
// context so a spent install deadline cannot skip it.
func Install(ctx context.Context, cfg Config, vol Volume, sw Switcher, certPEM, keyPEM []byte, want Info, orig Origin) (Result, error) {
	if _, found, err := ReadJournal(ctx, vol); err != nil {
		return Result{Outcome: OutcomeFailed, Reason: "read_journal"}, fmt.Errorf("mail cert: read install journal: %w", err)
	} else if found {
		return Result{Outcome: OutcomeFailed, Reason: "in_progress"}, ErrInstallInProgress
	}
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
	j := Journal{Phase: PhaseStaging, StartedAt: time.Now().UTC(), NewSHA256: want.SHA256, HadPrev: hadPrev, Origin: orig}
	if err := writeJournal(ctx, vol, j); err != nil {
		res.Outcome, res.Reason = OutcomeFailed, "journal"
		return res, err
	}
	abort := func(reason string, cause error) (Result, error) {
		res.Outcome, res.Reason = OutcomeFailed, reason
		if err := clearStaging(ctx, cfg, vol); err != nil {
			return res, errors.Join(cause, err)
		}
		return res, cause
	}
	if hadPrev {
		if err := writePair(ctx, vol, PrevCertPath, PrevKeyPath, prevCert, prevKey); err != nil {
			return abort("save_previous", err)
		}
	}
	if err := writePair(ctx, vol, NewCertPath, NewKeyPath, certPEM, keyPEM); err != nil {
		return abort("stage", err)
	}
	if err := verifyStaged(ctx, vol, certPEM, keyPEM); err != nil {
		return abort("stage", err)
	}
	j.Phase = PhaseSwapping
	if err := writeJournal(ctx, vol, j); err != nil {
		return abort("journal", err)
	}
	// Both halves are written back to back from checked bytes, so a crash leaves a mismatch only in this short window, and the journal covers it.
	if err := writePair(ctx, vol, CertPath, KeyPath, certPEM, keyPEM); err != nil {
		return rollback(ctx, cfg, vol, sw, res, prevCert, prevKey, false, "write", err)
	}
	if err := sw.Activate(ctx); err != nil {
		return rollback(ctx, cfg, vol, sw, res, prevCert, prevKey, true, "restart", err)
	}
	served, err := waitServed(ctx, cfg, sw, want)
	if err != nil {
		return rollback(ctx, cfg, vol, sw, res, prevCert, prevKey, true, "verify", err)
	}
	res.Outcome, res.Served = OutcomeInstalled, served
	if err := clearStaging(ctx, cfg, vol); err != nil {
		// The new pair is served; Recover keeps a served pair, so a left journal is harmless.
		res.Reason = "journal_left"
	}
	return res, nil
}

// Recover finishes an install found through its journal. A pair that is
// already served and on disk is kept; anything else goes back to the
// previous pair and setting. A nil error means the volume is consistent again.
func Recover(ctx context.Context, cfg Config, vol Volume, sw Switcher, j Journal) (Result, error) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.RollbackTimeout)
	defer cancel()
	res := Result{HadPrev: j.HadPrev, Reason: "interrupted"}
	if j.Phase == PhaseStaging {
		res.Outcome = OutcomeRolledBack
		return res, clearStaging(rctx, cfg, vol)
	}
	if cur, ok, err := vol.Read(rctx, CertPath); err == nil && ok && j.NewSHA256 != "" && leafSHA256(cur) == j.NewSHA256 {
		if served, serr := sw.Served(rctx); serr == nil && served.SHA256 == j.NewSHA256 {
			res.Outcome, res.Served = OutcomeInstalled, served
			return res, clearStaging(rctx, cfg, vol)
		}
	}
	var prevCert, prevKey []byte
	if j.HadPrev {
		c, okc, errc := vol.Read(rctx, PrevCertPath)
		k, okk, errk := vol.Read(rctx, PrevKeyPath)
		if err := errors.Join(errc, errk); err != nil || !okc || !okk {
			res.Outcome = OutcomeRollbackFailed
			return res, errors.Join(ErrInterrupted, errors.New("mail cert: the previous pair to restore is missing"), err)
		}
		prevCert, prevKey = c, k
	}
	res, err := rollback(rctx, cfg, vol, sw, res, prevCert, prevKey, true, "interrupted", ErrInterrupted)
	if res.Outcome == OutcomeRolledBack {
		return res, nil
	}
	return res, err
}

func leafSHA256(certPEM []byte) string {
	b, _ := pem.Decode(certPEM)
	if b == nil {
		return ""
	}
	sum := sha256.Sum256(b.Bytes)
	return hex.EncodeToString(sum[:])
}

// verifyStaged reads the staged pair back and checks it is byte for byte what was sent.
func verifyStaged(ctx context.Context, vol Volume, certPEM, keyPEM []byte) error {
	c, okc, err := vol.Read(ctx, NewCertPath)
	if err != nil {
		return fmt.Errorf("mail cert: read staged certificate: %w", err)
	}
	k, okk, err := vol.Read(ctx, NewKeyPath)
	if err != nil {
		return fmt.Errorf("mail cert: read staged key: %w", err)
	}
	if !okc || !okk || string(c) != string(certPEM) || string(k) != string(keyPEM) {
		return errors.New("mail cert: the staged pair on the node does not match the upload")
	}
	return nil
}

// clearStaging removes the staged pair, then the journal, so the journal is the last thing to go.
func clearStaging(ctx context.Context, cfg Config, vol Volume) error {
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.RollbackTimeout)
	defer cancel()
	if err := errors.Join(vol.Remove(cctx, NewKeyPath), vol.Remove(cctx, NewCertPath)); err != nil {
		return fmt.Errorf("mail cert: remove staged pair: %w", err)
	}
	if err := vol.Remove(cctx, JournalPath); err != nil {
		return fmt.Errorf("mail cert: remove install journal: %w", err)
	}
	return nil
}

// writePair writes the key first and checks its mode before the certificate.
// A key that landed too wide is deleted, and a failed delete is fatal.
func writePair(ctx context.Context, vol Volume, certPath, keyPath string, certPEM, keyPEM []byte) error {
	if err := vol.Write(ctx, keyPath, keyPEM, true); err != nil {
		return fmt.Errorf("mail cert: write key: %w", err)
	}
	mode, err := vol.Mode(ctx, keyPath)
	if err != nil || mode.Perm()&0o077 != 0 {
		if rerr := vol.Remove(ctx, keyPath); rerr != nil {
			return errors.Join(ErrKeyRemoveFailed, rerr)
		}
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

func restore(ctx context.Context, vol Volume, hadPrev bool, prevCert, prevKey []byte) error {
	if hadPrev {
		return writePair(ctx, vol, CertPath, KeyPath, prevCert, prevKey)
	}
	return errors.Join(vol.Remove(ctx, KeyPath), vol.Remove(ctx, CertPath))
}

// rollback restores the previous pair and setting on a fresh bounded context.
// When that fails the journal stays, so the next call retries the recovery.
func rollback(ctx context.Context, cfg Config, vol Volume, sw Switcher, res Result, prevCert, prevKey []byte, activated bool, reason string, cause error) (Result, error) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.RollbackTimeout)
	defer cancel()
	res.Reason = reason
	var errs []error
	if err := restore(rctx, vol, res.HadPrev, prevCert, prevKey); err != nil {
		errs = append(errs, fmt.Errorf("mail cert: restore previous pair: %w", err))
	}
	if activated {
		if err := sw.Revert(rctx); err != nil {
			errs = append(errs, fmt.Errorf("mail cert: rollback restart: %w", err))
		}
	}
	if len(errs) > 0 {
		res.Outcome = OutcomeRollbackFailed
		return res, errors.Join(append([]error{cause}, errs...)...)
	}
	res.Outcome = OutcomeRolledBack
	if err := clearStaging(rctx, cfg, vol); err != nil {
		return res, errors.Join(cause, err)
	}
	return res, cause
}

// waitServed polls until the served leaf has want's fingerprint, not only its serial.
func waitServed(ctx context.Context, cfg Config, sw Switcher, want Info) (Info, error) {
	deadline := time.Now().Add(cfg.VerifyTimeout)
	var last error
	for {
		got, err := sw.Served(ctx)
		if err == nil && got.SHA256 == want.SHA256 && got.Serial == want.Serial {
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
