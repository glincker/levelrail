package email

import (
	"context"
	"fmt"
	"net/smtp"
)

// smtpSender sends via net/smtp, moved here unchanged from
// alerting.sendPlainEmail (this package's predecessor). net/smtp.SendMail
// has no context.Context parameter to plumb ctx's cancellation through: a
// hung SMTP connection isn't cancellable the way an HTTP send is, a real,
// documented limitation, not an oversight.
type smtpSender struct {
	cfg SMTPConfig
}

func (s smtpSender) Send(_ context.Context, to, subject, body string) error {
	msg := fmt.Sprintf("To: %s\r\nFrom: %s\r\nSubject: %s\r\n\r\n%s\r\n", to, s.cfg.From, subject, body)

	var auth smtp.Auth
	if s.cfg.Username != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	}
	if err := smtp.SendMail(s.cfg.Addr, auth, s.cfg.From, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
}
