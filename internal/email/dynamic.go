package email

import (
	"context"
	"fmt"
	"strings"
)

// ConfigLoader resolves the current Config on every call, never cached:
// settings can change at runtime.
type ConfigLoader func(ctx context.Context) (Config, error)

// DynamicSender adapts a ConfigLoader into a Sender, so a settings
// change takes effect on the very next send with no restart required.
type DynamicSender struct {
	load ConfigLoader
}

// NewDynamicSender builds a DynamicSender. load must not be nil.
func NewDynamicSender(load ConfigLoader) *DynamicSender {
	return &DynamicSender{load: load}
}

// Send implements Sender. It rejects a CR or LF in to or subject before
// any backend runs, since either could start an injected header.
func (d *DynamicSender) Send(ctx context.Context, to, subject, body string) error {
	if err := rejectHeaderInjection(to, subject); err != nil {
		return err
	}
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

// rejectHeaderInjection reports an error if to or subject carries a CR
// or LF.
func rejectHeaderInjection(to, subject string) error {
	if strings.ContainsAny(to, "\r\n") {
		return fmt.Errorf("email: to address contains a newline")
	}
	if strings.ContainsAny(subject, "\r\n") {
		return fmt.Errorf("email: subject contains a newline")
	}
	return nil
}
