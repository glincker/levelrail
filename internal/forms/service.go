package forms

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	notifyTimeout    = 15 * time.Second
	pruneEvery       = time.Hour
	noticeValueLimit = 120
	noticeFieldLimit = 8
)

// Store is the persistence surface Submit needs.
type Store interface {
	AddFormSubmission(ctx context.Context, s store.FormSubmission, maxStored int) (store.FormSubmission, error)
	PruneFormSubmissions(ctx context.Context, cutoff time.Time) (int64, error)
}

// Notifier tells the form's channels about a new, non-spam submission.
type Notifier interface {
	Notify(ctx context.Context, f store.Form, s store.FormSubmission) error
}

// Service stores submissions and fires notifications.
type Service struct {
	store    Store
	notifier Notifier
	cfg      Config
	logger   *slog.Logger
	// Dispatch runs notification work; tests replace it to run inline.
	Dispatch  func(func())
	Now       func() time.Time
	lastPrune atomic.Int64
}

// NewService builds a Service; notifier may be nil to disable notifications.
func NewService(st Store, n Notifier, cfg Config, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: st, notifier: n, cfg: cfg, logger: logger, Dispatch: func(fn func()) { go fn() }, Now: time.Now}
}

// Config returns the limits the service was built with.
func (s *Service) Config() Config { return s.cfg }

// Submit classifies and stores one post. A failed notification never fails the submission:
// the row is already saved, so it is logged and the visitor still gets success.
func (s *Service) Submit(ctx context.Context, f store.Form, fields map[string]string, clientIP string) (store.FormSubmission, error) {
	return s.submit(ctx, f, fields, clientIP, false)
}

// SubmitTest stores a dashboard test post, flagged as a test and never notified.
func (s *Service) SubmitTest(ctx context.Context, f store.Form, fields map[string]string) (store.FormSubmission, error) {
	return s.submit(ctx, f, fields, "", true)
}

func (s *Service) submit(ctx context.Context, f store.Form, fields map[string]string, clientIP string, test bool) (store.FormSubmission, error) {
	clean, spam, reason := Classify(fields, f.HoneypotField, s.cfg.SpamMaxLinks)
	now := s.Now().UTC()
	sub, err := s.store.AddFormSubmission(ctx, store.FormSubmission{
		FormID: f.ID, Fields: clean, Spam: spam, SpamReason: reason,
		IPHash: HashIP(f.IPSalt, clientIP), CreatedAt: now, Test: test,
	}, s.cfg.MaxStoredPerForm)
	if err != nil {
		return store.FormSubmission{}, fmt.Errorf("forms: store submission for %q: %w", f.ID, err)
	}
	s.maybePrune(ctx, now)
	if !spam && !test && s.notifier != nil && len(f.NotifyChannelIDs) > 0 {
		s.Dispatch(func() {
			nctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), notifyTimeout)
			defer cancel()
			if err := s.notifier.Notify(nctx, f, sub); err != nil {
				s.logger.Error("forms: notify failed", slog.String("form_id", f.ID), slog.Int64("submission_id", sub.ID), slog.String("error", err.Error()))
			}
		})
	}
	return sub, nil
}

func (s *Service) maybePrune(ctx context.Context, now time.Time) {
	if s.cfg.Retention <= 0 {
		return
	}
	last := s.lastPrune.Load()
	if now.UnixNano()-last < int64(pruneEvery) || !s.lastPrune.CompareAndSwap(last, now.UnixNano()) {
		return
	}
	if n, err := s.store.PruneFormSubmissions(ctx, now.Add(-s.cfg.Retention)); err != nil {
		s.logger.Error("forms: prune failed", slog.String("error", err.Error()))
	} else if n > 0 {
		s.logger.Info("forms: pruned expired submissions", slog.Int64("count", n))
	}
}

// NoticeText is the plain-text body sent to channels. Values are flattened and truncated
// so a submission cannot inject markup-length payloads or mentions into a chat channel.
func NoticeText(f store.Form, s store.FormSubmission, dashboardURL string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "New submission on form %q (app %s)", f.Name, f.AppName)
	keys := SortedKeys(s.Fields)
	for i, k := range keys {
		if i == noticeFieldLimit {
			fmt.Fprintf(&b, "\n... and %d more fields", len(keys)-noticeFieldLimit)
			break
		}
		fmt.Fprintf(&b, "\n%s: %s", flatten(k, noticeValueLimit), flatten(s.Fields[k], noticeValueLimit))
	}
	if dashboardURL != "" {
		b.WriteString("\n" + dashboardURL)
	}
	return b.String()
}

func flatten(v string, limit int) string {
	v = strings.Join(strings.Fields(strings.NewReplacer("@", "(at)", "<", "", ">", "").Replace(v)), " ")
	if r := []rune(v); len(r) > limit {
		return string(r[:limit]) + "..."
	}
	return v
}
