package compliance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Audit action names written by this package.
const (
	ActionAccessRevoked   = "compliance.access_revoked"
	ActionReviewCompleted = "compliance.access_review_completed"
	ActionPackGenerated   = "compliance.evidence_pack_generated"
)

const (
	defaultDueDays = 14
	maxRange       = 366 * 24 * time.Hour
)

var (
	// ErrUndecided is returned when a campaign still has rows without a decision.
	ErrUndecided = errors.New("compliance: every row needs a keep or revoke decision before completing")
	// ErrApplyFailed is returned when at least one revocation could not be applied.
	ErrApplyFailed = errors.New("compliance: some revocations could not be applied")
	// ErrInvalidRange is returned for an empty, inverted or oversized date range.
	ErrInvalidRange = errors.New("compliance: date range must be positive and at most 366 days")
	// ErrPackNotReady is returned when downloading a pack that is not complete.
	ErrPackNotReady = errors.New("compliance: evidence pack is not complete")
)

// Actor is the human or token performing a change.
type Actor struct{ Type, ID, Name string }

// Service owns access review campaigns and evidence pack generation.
type Service struct {
	db       *store.DB
	dir      string
	signer   *Signer
	security Source
	users    UserRemover
	logger   *slog.Logger
	now      func() time.Time
	ctx      context.Context
	wg       sync.WaitGroup
	// sections is replaceable so tests can inject a failing section.
	sections func(from, to time.Time) []Section
}

// NewService returns a service writing packs under dir. ctx bounds every
// background generation job.
func NewService(ctx context.Context, db *store.DB, dir string, signer *Signer, security Source, users UserRemover, logger *slog.Logger) *Service {
	s := &Service{db: db, dir: dir, signer: signer, security: security, users: users, logger: logger, now: time.Now, ctx: ctx}
	s.sections = s.DefaultSections
	return s
}

// Store exposes read access for the API's list and detail views.
func (s *Service) Store() *store.DB { return s.db }

// Wait blocks until running generation jobs finish.
func (s *Service) Wait() { s.wg.Wait() }

// Recover marks packs a previous process left running as failed.
func (s *Service) Recover(ctx context.Context) error {
	return s.db.FailRunningEvidencePacks(ctx, s.now().UTC().Format(time.RFC3339), "interrupted by a restart")
}

func (s *Service) audit(ctx context.Context, a Actor, action, path string, status int) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	e := store.AuditEntry{ID: id, ActorType: a.Type, ActorID: a.ID, ActorName: a.Name, Ability: "root",
		Method: "POST", Path: path, StatusCode: status, CreatedAt: store.FormatAuditTime(s.now()),
		ClientKind: "api", Action: action}
	if err := s.db.SaveAuditEntry(ctx, e); err != nil {
		s.logger.Warn("compliance: audit write failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}

// CreateCampaign snapshots current access into a new open campaign.
func (s *Service) CreateCampaign(ctx context.Context, name string, dueDays, repeatDays int, by Actor) (store.AccessReviewCampaign, error) {
	if dueDays <= 0 {
		dueDays = defaultDueDays
	}
	if name == "" {
		name = "Access review " + s.now().UTC().Format("2006-01-02")
	}
	items, err := Snapshot(ctx, s.db)
	if err != nil {
		return store.AccessReviewCampaign{}, err
	}
	id, err := newID("arc_")
	if err != nil {
		return store.AccessReviewCampaign{}, err
	}
	now := s.now().UTC()
	c := store.AccessReviewCampaign{ID: id, Name: name, Status: store.AccessReviewOpen, DueDays: dueDays, RepeatDays: repeatDays,
		DueAt: now.AddDate(0, 0, dueDays).Format(time.RFC3339), CreatedBy: by.Name, CreatedAt: now.Format(time.RFC3339)}
	if err := s.db.CreateAccessReviewCampaign(ctx, c, items); err != nil {
		return c, fmt.Errorf("compliance: %w", err)
	}
	return c, nil
}

// Decide records keep or revoke for one row of an open campaign.
func (s *Service) Decide(ctx context.Context, campaignID, itemID, decision string, by Actor) error {
	if decision != store.AccessDecisionKeep && decision != store.AccessDecisionRevoke {
		return fmt.Errorf("compliance: decision must be %s or %s", store.AccessDecisionKeep, store.AccessDecisionRevoke)
	}
	c, err := s.db.GetAccessReviewCampaign(ctx, campaignID)
	if err != nil {
		return err
	}
	if c.Status != store.AccessReviewOpen {
		return errors.New("compliance: the campaign is already completed")
	}
	return s.db.DecideAccessReviewItem(ctx, campaignID, itemID, decision, by.Name, s.now().UTC().Format(time.RFC3339))
}

// CompleteResult says what completing a campaign did.
type CompleteResult struct {
	Revoked  int
	Failures []store.AccessReviewItem
}

// Complete applies every revoke decision, then closes the campaign. When
// any revocation fails the campaign stays open, successes stay applied,
// and the failures are returned for the reviewer to retry or keep.
func (s *Service) Complete(ctx context.Context, campaignID string, by Actor) (CompleteResult, error) {
	var res CompleteResult
	c, err := s.db.GetAccessReviewCampaign(ctx, campaignID)
	if err != nil {
		return res, err
	}
	if c.Status != store.AccessReviewOpen {
		return res, errors.New("compliance: the campaign is already completed")
	}
	items, err := s.db.ListAccessReviewItems(ctx, campaignID)
	if err != nil {
		return res, fmt.Errorf("compliance: %w", err)
	}
	for _, it := range items {
		if it.Decision == "" {
			return res, ErrUndecided
		}
	}
	now := s.now().UTC().Format(time.RFC3339)
	for _, it := range items {
		if it.Decision != store.AccessDecisionRevoke || it.AppliedAt != "" {
			continue
		}
		if err := s.applyRevocation(ctx, it, by.ID); err != nil {
			it.ApplyError = err.Error()
			if serr := s.db.SetAccessReviewItemApplied(ctx, it.ID, "", it.ApplyError); serr != nil {
				return res, fmt.Errorf("compliance: %w", serr)
			}
			res.Failures = append(res.Failures, it)
			continue
		}
		if err := s.db.SetAccessReviewItemApplied(ctx, it.ID, now, ""); err != nil {
			return res, fmt.Errorf("compliance: %w", err)
		}
		res.Revoked++
		s.audit(ctx, by, ActionAccessRevoked, "/api/v1/compliance/access-reviews/"+campaignID+"#"+it.Kind+":"+it.SubjectID, 200)
	}
	if len(res.Failures) > 0 {
		return res, ErrApplyFailed
	}
	if err := s.db.CompleteAccessReviewCampaign(ctx, campaignID, now); err != nil {
		return res, fmt.Errorf("compliance: %w", err)
	}
	s.audit(ctx, by, ActionReviewCompleted, "/api/v1/compliance/access-reviews/"+campaignID, 200)
	return res, nil
}

// Overdue returns open campaigns past their due date.
func (s *Service) Overdue(ctx context.Context) ([]store.AccessReviewCampaign, error) {
	all, err := s.db.ListAccessReviewCampaigns(ctx)
	if err != nil {
		return nil, fmt.Errorf("compliance: %w", err)
	}
	now := s.now().UTC()
	var out []store.AccessReviewCampaign
	for _, c := range all {
		if due, err := time.Parse(time.RFC3339, c.DueAt); err == nil && c.Status == store.AccessReviewOpen && now.After(due) {
			out = append(out, c)
		}
	}
	return out, nil
}

// RunScheduler opens the next campaign of a repeating series once the
// previous one is completed and its interval has elapsed.
func (s *Service) RunScheduler(ctx context.Context, every time.Duration) {
	_ = footprint.Every(ctx, "access review scheduler", every, s.logger, s.scheduleOnce)
}

func (s *Service) scheduleOnce(ctx context.Context) error {
	all, err := s.db.ListAccessReviewCampaigns(ctx)
	if err != nil {
		return err
	}
	for _, c := range all {
		if c.RepeatDays <= 0 {
			continue
		}
		created, perr := time.Parse(time.RFC3339, c.CreatedAt)
		if c.Status == store.AccessReviewCompleted && perr == nil && !s.now().Before(created.AddDate(0, 0, c.RepeatDays)) {
			_, err := s.CreateCampaign(ctx, "", c.DueDays, c.RepeatDays, Actor{Type: "system", Name: "scheduler"})
			return err
		}
		return nil
	}
	return nil
}

// PackPath is where a complete pack's archive lives.
func (s *Service) PackPath(p store.EvidencePack) string {
	return filepath.Join(s.dir, filepath.Base(p.FileName))
}

// StartPack records a running pack and generates it in the background.
func (s *Service) StartPack(ctx context.Context, from, to time.Time, by Actor) (store.EvidencePack, error) {
	if !to.After(from) || to.Sub(from) > maxRange {
		return store.EvidencePack{}, ErrInvalidRange
	}
	if s.signer == nil {
		return store.EvidencePack{}, errors.New("compliance: evidence packs need a master key to sign with")
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return store.EvidencePack{}, fmt.Errorf("compliance: create pack directory: %w", err)
	}
	id, err := newID("evp_")
	if err != nil {
		return store.EvidencePack{}, err
	}
	p := store.EvidencePack{ID: id, RangeFrom: from.UTC().Format(time.RFC3339), RangeTo: to.UTC().Format(time.RFC3339),
		Status: store.EvidencePackRunning, Step: "queued", CreatedBy: by.Name, CreatedAt: s.now().UTC().Format(time.RFC3339)}
	if err := s.db.CreateEvidencePack(ctx, p); err != nil {
		return p, fmt.Errorf("compliance: %w", err)
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.generate(p, from, to, by)
	}()
	return p, nil
}

func (s *Service) generate(p store.EvidencePack, from, to time.Time, by Actor) {
	ctx := s.ctx
	final := p.ID + ".zip"
	tmp := filepath.Join(s.dir, final+".partial")
	fail := func(err error) {
		_ = os.Remove(tmp)
		s.logger.Warn("compliance: evidence pack failed", slog.String("pack", p.ID), slog.String("error", err.Error()))
		p.Status, p.Error, p.FinishedAt = store.EvidencePackFailed, err.Error(), s.now().UTC().Format(time.RFC3339)
		if ferr := s.db.FinishEvidencePack(context.WithoutCancel(ctx), p); ferr != nil {
			s.logger.Error("compliance: could not record pack failure", slog.String("pack", p.ID), slog.String("error", ferr.Error()))
		}
	}
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // path is the service directory plus a generated id
	if err != nil {
		fail(err)
		return
	}
	meta := Meta{PackID: p.ID, RangeFrom: from, RangeTo: to, Now: s.now()}
	progress := func(step string, done, total int) {
		_ = s.db.SetEvidencePackProgress(ctx, p.ID, step, done*100/total)
	}
	err = Build(ctx, f, meta, s.sections(from, to), s.signer, progress)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		fail(err)
		return
	}
	sum, size, err := hashFile(tmp)
	if err != nil {
		fail(err)
		return
	}
	finalPath := filepath.Join(s.dir, final)
	if err := os.Rename(tmp, finalPath); err != nil {
		fail(err)
		return
	}
	p.Status, p.Step, p.Progress, p.FileName, p.SizeBytes, p.SHA256 = store.EvidencePackComplete, "done", 100, final, size, sum
	p.FinishedAt = s.now().UTC().Format(time.RFC3339)
	if err := s.db.FinishEvidencePack(context.WithoutCancel(ctx), p); err != nil {
		_ = os.Remove(finalPath)
		fail(err)
		return
	}
	s.audit(context.WithoutCancel(ctx), by, ActionPackGenerated, "/api/v1/compliance/evidence-packs/"+p.ID, 200)
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path) //nolint:gosec // path is the service directory plus a generated id
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", n, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// VerifyPack re-checks a stored pack against its manifest and signature.
func (s *Service) VerifyPack(ctx context.Context, id string) (Report, error) {
	p, err := s.db.GetEvidencePack(ctx, id)
	if err != nil {
		return Report{}, err
	}
	if p.Status != store.EvidencePackComplete {
		return Report{}, ErrPackNotReady
	}
	f, err := os.Open(s.PackPath(p))
	if err != nil {
		return Report{}, fmt.Errorf("compliance: open pack: %w", err)
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return Report{}, fmt.Errorf("compliance: stat pack: %w", err)
	}
	return Verify(f, st.Size(), s.signer)
}
