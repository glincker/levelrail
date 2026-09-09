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

// Send implements Sender. Every caller (invite emails, password resets,
// deploy/alert notifications) ultimately funnels through here regardless
// of backend, so this is the one place that rejects a CR or LF in to or
// subject: smtpSender.Send builds a raw "To: %s\r\nSubject: %s\r\n..."
// header block via fmt.Sprintf, and neither to (an operator-entered
// invite/user email, internal/api/invites.go and users.go do no format
// validation) nor subject is otherwise guaranteed free of \r\n, which
// would otherwise inject arbitrary extra headers (a Bcc:, for example)
// into the outgoing message.
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
// or LF, either of which would let a caller inject an arbitrary extra
// header (or terminate the header block early) into smtpSender's raw
// message construction.
func rejectHeaderInjection(to, subject string) error {
	if strings.ContainsAny(to, "\r\n") {
		return fmt.Errorf("email: to address contains a newline")
	}
	if strings.ContainsAny(subject, "\r\n") {
		return fmt.Errorf("email: subject contains a newline")
	}
	return nil
}
