// Package mailcast sends broadcasts: a queue of per-recipient messages
// processed in rate-limited batches, resumable after a restart and
// cancellable while sending.
package mailcast

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Store is the persistence the runner needs; *store.DB satisfies it.
type Store interface {
	GetMailBroadcast(ctx context.Context, id string) (*store.MailBroadcast, error)
	UpdateMailBroadcast(ctx context.Context, b store.MailBroadcast) error
	UpdateMailBroadcastProgress(ctx context.Context, id string, sent, failed, suppressed int) error
	FinishMailBroadcast(ctx context.Context, b store.MailBroadcast) error
	ListMailBroadcastsByStatus(ctx context.Context, statuses ...string) ([]store.MailBroadcast, error)
	ListMailRecipients(ctx context.Context, broadcastID, status string, limit int) ([]store.MailRecipient, error)
	SetMailRecipientStatus(ctx context.Context, broadcastID, email, status, errText string) error
	CancelPendingMailRecipients(ctx context.Context, broadcastID string) error
	ListMailSuppressions(ctx context.Context) ([]store.MailSuppression, error)
}

// RenderFunc builds the message for one recipient.
type RenderFunc func(ctx context.Context, b store.MailBroadcast, r store.MailRecipient) (email.Message, error)

// SendFunc delivers one message.
type SendFunc func(ctx context.Context, m email.Message) error

// Runner drives broadcasts. Create it with New.
type Runner struct {
	Store  Store
	Render RenderFunc
	Send   SendFunc
	Logger *slog.Logger
	// Delay is the pause between two messages, the sending rate limit.
	Delay time.Duration
	// Batch is how many recipients are loaded between status re-checks.
	Batch int
	// SendTimeout bounds one message.
	SendTimeout time.Duration

	mu      sync.Mutex
	running map[string]context.CancelFunc
}

// New builds a Runner with conservative defaults.
func New(s Store, render RenderFunc, send SendFunc, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{Store: s, Render: render, Send: send, Logger: logger, Delay: 200 * time.Millisecond, Batch: 50, SendTimeout: 30 * time.Second, running: map[string]context.CancelFunc{}}
}

// Start runs a broadcast in the background; a second Start for the same id is a no-op.
func (r *Runner) Start(parent context.Context, id string) {
	r.mu.Lock()
	if _, ok := r.running[id]; ok {
		r.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	r.running[id] = cancel
	r.mu.Unlock()
	go func() {
		defer func() {
			r.mu.Lock()
			delete(r.running, id)
			r.mu.Unlock()
			cancel()
		}()
		if err := r.Run(ctx, id); err != nil && !errors.Is(err, context.Canceled) {
			r.Logger.Error("mailcast: broadcast run failed", slog.String("broadcast_id", id), slog.String("error", err.Error()))
		}
	}()
}

// Cancel stops a broadcast: unsent recipients are marked cancelled.
func (r *Runner) Cancel(ctx context.Context, id string) error {
	b, err := r.Store.GetMailBroadcast(ctx, id)
	if err != nil {
		return err
	}
	if b.Status != store.MailBroadcastScheduled && b.Status != store.MailBroadcastSending {
		return fmt.Errorf("broadcast is %s and cannot be cancelled", b.Status)
	}
	b.Status = store.MailBroadcastCancelled
	b.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := r.Store.UpdateMailBroadcast(ctx, *b); err != nil {
		return fmt.Errorf("cancel broadcast: %w", err)
	}
	if err := r.Store.CancelPendingMailRecipients(ctx, id); err != nil {
		return fmt.Errorf("cancel recipients: %w", err)
	}
	r.mu.Lock()
	if c, ok := r.running[id]; ok {
		c()
	}
	r.mu.Unlock()
	return nil
}

// Tick starts every scheduled broadcast that is due and resumes any that
// were interrupted while sending. Call it on an interval.
func (r *Runner) Tick(ctx context.Context, now time.Time) {
	list, err := r.Store.ListMailBroadcastsByStatus(ctx, store.MailBroadcastScheduled, store.MailBroadcastSending)
	if err != nil {
		r.Logger.Warn("mailcast: list broadcasts failed", slog.String("error", err.Error()))
		return
	}
	for _, b := range list {
		if b.Status == store.MailBroadcastScheduled && b.ScheduledAt != "" {
			at, perr := time.Parse(time.RFC3339Nano, b.ScheduledAt)
			if perr == nil && at.After(now) {
				continue
			}
		}
		r.Start(ctx, b.ID)
	}
}

// Run sends every pending recipient of a broadcast, returning when done,
// cancelled or ctx ends.
func (r *Runner) Run(ctx context.Context, id string) error {
	b, err := r.Store.GetMailBroadcast(ctx, id)
	if err != nil {
		return err
	}
	if b.Status != store.MailBroadcastScheduled && b.Status != store.MailBroadcastSending {
		return nil
	}
	b.Status = store.MailBroadcastSending
	if b.StartedAt == "" {
		b.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := r.Store.UpdateMailBroadcast(ctx, *b); err != nil {
		return err
	}
	for {
		cur, err := r.Store.GetMailBroadcast(ctx, id)
		if err != nil {
			return err
		}
		if cur.Status != store.MailBroadcastSending {
			return nil
		}
		batch, err := r.Store.ListMailRecipients(ctx, id, store.MailRecipientPending, r.Batch)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return r.finish(ctx, cur)
		}
		suppressed, err := r.suppressed(ctx)
		if err != nil {
			return err
		}
		for _, rc := range batch {
			if err := ctx.Err(); err != nil {
				return err
			}
			r.deliver(ctx, cur, rc, suppressed)
			if err := r.Store.UpdateMailBroadcastProgress(ctx, id, cur.Sent, cur.Failed, cur.Suppressed); err != nil {
				return err
			}
			if !sleep(ctx, r.Delay) {
				return ctx.Err()
			}
		}
	}
}

func (r *Runner) deliver(ctx context.Context, b *store.MailBroadcast, rc store.MailRecipient, suppressed map[string]bool) {
	if suppressed[rc.Email] {
		b.Suppressed++
		r.record(ctx, b.ID, rc.Email, store.MailRecipientSuppressed, "")
		return
	}
	msg, err := r.Render(ctx, *b, rc)
	if err == nil {
		sctx, cancel := context.WithTimeout(ctx, r.SendTimeout)
		err = r.Send(sctx, msg)
		cancel()
	}
	if err != nil {
		b.Failed++
		r.record(ctx, b.ID, rc.Email, store.MailRecipientFailed, err.Error())
		return
	}
	b.Sent++
	r.record(ctx, b.ID, rc.Email, store.MailRecipientSent, "")
}

func (r *Runner) record(ctx context.Context, id, addr, status, errText string) {
	if err := r.Store.SetMailRecipientStatus(ctx, id, addr, status, errText); err != nil {
		r.Logger.Warn("mailcast: record recipient failed", slog.String("broadcast_id", id), slog.String("error", err.Error()))
	}
}

func (r *Runner) finish(ctx context.Context, b *store.MailBroadcast) error {
	b.Status = store.MailBroadcastCompleted
	if b.Sent == 0 && b.Failed > 0 {
		b.Status = store.MailBroadcastFailed
		b.Error = "no message could be delivered"
	}
	b.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return r.Store.FinishMailBroadcast(ctx, *b)
}

func (r *Runner) suppressed(ctx context.Context) (map[string]bool, error) {
	list, err := r.Store.ListMailSuppressions(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(list))
	for _, s := range list {
		out[s.Email] = true
	}
	return out, nil
}

func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
