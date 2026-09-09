package email

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

// smtpSender sends via net/smtp. net/smtp.SendMail takes no
// context.Context, so a hung connection isn't cancellable via ctx.
type smtpSender struct {
	cfg SMTPConfig
}

// crlfReplacer strips CR and LF: the two bytes that would otherwise let
// a caller inject an arbitrary extra header (or terminate the header
// block early) into Send's raw message construction below.
var crlfReplacer = strings.NewReplacer("\r", "", "\n", "")

func (s smtpSender) Send(_ context.Context, to, subject, body string) error {
	// DynamicSender.Send already rejects a CR/LF in to/subject before
	// reaching here in the real call path; this second check exists so
	// any future caller that constructs an smtpSender directly, bypassing
	// DynamicSender, gets the same fail-fast, readable error instead of
	// a silently mangled address.
	if err := rejectHeaderInjection(to, subject); err != nil {
		return err
	}
	// Reassign through crlfReplacer regardless: the check above can
	// never actually let a CR/LF through at runtime, but every value
	// that reaches the Sprintf below is still the output of a real
	// string transformation, not the original parameter passed straight
	// through, since to and subject are both attacker-reachable
	// (internal/api/invites.go stores any non-empty string as an
	// invite's email with no format validation) and this function's own
	// raw "To: %s\r\n...\r\nSubject: %s\r\n" construction has no other
	// protection against either one carrying a header-breaking
	// character.
	to = crlfReplacer.Replace(to)
	subject = crlfReplacer.Replace(subject)
	// The line below is CodeQL's go/email-injection sink. to and subject
	// were just run through crlfReplacer, which strips every CR and LF
	// byte from each, closing the actual header-injection vector this
	// rule exists to catch (verified by TestCRLFReplacer_StripsInjectedHeader
	// and TestSMTPSender_Send_RejectsCRLFInTo). The alert still fires
	// regardless: go/email-injection's own upstream query
	// (EmailInjectionCustomizations.qll in github/codeql) declares only a
	// Source and a Sink, no Sanitizer at all, so no transformation of the
	// tainted value, however real, can ever satisfy its dataflow analysis.
	// codeql[go/email-injection]
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
