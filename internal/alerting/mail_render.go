package alerting

import (
	"context"
	"errors"
	"time"

	"github.com/GLINCKER/levelrail/internal/email"
)

// MailRenderFunc renders one mailtemplates event for a recipient; ok is false when no template applies.
type MailRenderFunc func(ctx context.Context, key, to string, values map[string]string) (email.Message, bool)

// TemplatedSender is an email.Sender that can also render the branded event templates.
type TemplatedSender struct {
	email.Sender
	Render MailRenderFunc
}

// SendMessage keeps the HTML part and headers when the wrapped sender supports them.
func (t TemplatedSender) SendMessage(ctx context.Context, m email.Message) error {
	return email.SendMessage(ctx, t.Sender, m)
}

type mailRenderer interface {
	RenderMail(ctx context.Context, key, to string, values map[string]string) (email.Message, bool)
}

// RenderMail implements mailRenderer.
func (t TemplatedSender) RenderMail(ctx context.Context, key, to string, values map[string]string) (email.Message, bool) {
	if t.Render == nil {
		return email.Message{}, false
	}
	return t.Render(ctx, key, to, values)
}

// sendTemplated sends the rich template when the sender offers one, else the plain subject and body.
func sendTemplated(ctx context.Context, sender email.Sender, key, to string, values map[string]string, subject, body string) error {
	if r, ok := sender.(mailRenderer); ok {
		if m, ok := r.RenderMail(ctx, key, to, values); ok {
			return sendMessageWithRetry(ctx, sender, m)
		}
	}
	return sendEmailWithRetry(ctx, sender, to, subject, body)
}

func sendMessageWithRetry(ctx context.Context, sender email.Sender, m email.Message) error {
	var lastErr error
	delay := notifyRetryBaseDelay
	for attempt := 1; attempt <= notifyMaxAttempts; attempt++ {
		lastErr = email.SendMessage(ctx, sender, m)
		if lastErr == nil || attempt == notifyMaxAttempts || !isRetryableEmailError(lastErr) {
			return lastErr
		}
		if err := sleepCtx(ctx, delay); err != nil {
			return errors.Join(err, lastErr)
		}
		delay *= 2
	}
	return lastErr
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
