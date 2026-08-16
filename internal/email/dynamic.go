package email

import (
	"context"
	"fmt"
)

// ConfigLoader resolves the current Config on every call: an operator's
// settings can change at runtime (the platform-wide settings row PUT
// /api/v1/settings/email writes to), and this package never caches a
// decision across calls, the same "re-evaluate fresh, never a cached
// decision" rule internal/api's requireAbility already applies to a
// bearer token's abilities.
type ConfigLoader func(ctx context.Context) (Config, error)

// DynamicSender adapts a ConfigLoader into a Sender: every Send call
// resolves the current Config first, so a control plane whose operator
// just changed the email settings picks that up on the very next send,
// no restart required. cmd/levelrail/main.go builds the one real
// ConfigLoader (settings row, falling back to APP_SMTP_* env vars) and
// shares a single *DynamicSender between internal/alerting and
// internal/api.
type DynamicSender struct {
	load ConfigLoader
}

// NewDynamicSender builds a DynamicSender. load must not be nil.
func NewDynamicSender(load ConfigLoader) *DynamicSender {
	return &DynamicSender{load: load}
}

// Send implements Sender.
func (d *DynamicSender) Send(ctx context.Context, to, subject, body string) error {
	cfg, err := d.load(ctx)
	if err != nil {
		return fmt.Errorf("email: load config: %w", err)
	}
	sender, err := NewSender(cfg)
	if err != nil {
		return err
	}
	return sender.Send(ctx, to, subject, body)
}
