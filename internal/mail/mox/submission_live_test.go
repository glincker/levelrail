package mox

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/mail"
)

// TestLiveMoxSystemEmailSubmission proves the system email path against a throwaway mox: a generated
// noreply account submits over STARTTLS to the private listener with a pinned certificate. It needs
// MOX_LIVE_URL, MOX_LIVE_PASSWORD_FILE and MOX_LIVE_SUBMISSION (host:587), and is skipped otherwise.
func TestLiveMoxSystemEmailSubmission(t *testing.T) {
	base, pwFile, sub := os.Getenv("MOX_LIVE_URL"), os.Getenv("MOX_LIVE_PASSWORD_FILE"), os.Getenv("MOX_LIVE_SUBMISSION")
	domain := os.Getenv("MOX_LIVE_DOMAIN")
	if base == "" || pwFile == "" || sub == "" || domain == "" {
		t.Skip("set MOX_LIVE_URL, MOX_LIVE_PASSWORD_FILE, MOX_LIVE_SUBMISSION and MOX_LIVE_DOMAIN to run against a throwaway mox")
	}
	admin, err := os.ReadFile(pwFile) //nolint:gosec // operator supplied path for an opt-in live test
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(Config{BaseURL: base, Credentials: creds{pw: strings.TrimSpace(string(admin))}, HTTPClient: &http.Client{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	addr := "noreply@" + domain
	pw, err := mail.CreateAccount(ctx, c, addr, 0)
	if err != nil {
		t.Fatalf("create noreply: %v", err)
	}
	t.Cleanup(func() { _ = c.RemoveAccount(context.Background(), addr) })

	host, _, _ := net.SplitHostPort(sub)
	var pin string
	probe := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: true, //nolint:gosec // trust on first use to learn the pin
		VerifyConnection: func(cs tls.ConnectionState) error {
			sum := sha256.Sum256(cs.PeerCertificates[0].Raw)
			pin = hex.EncodeToString(sum[:])
			return nil
		}}
	if err := submit(sub, host, probe, addr, pw, ""); err != nil {
		t.Fatalf("probe: %v", err)
	}
	pinned := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: true, //nolint:gosec // replaced by the leaf pin below
		VerifyConnection: func(cs tls.ConnectionState) error {
			sum := sha256.Sum256(cs.PeerCertificates[0].Raw)
			if hex.EncodeToString(sum[:]) != pin {
				return errors.New("pin mismatch")
			}
			return nil
		}}
	if err := submit(sub, host, pinned, addr, pw, "postmaster@"+domain); err != nil {
		t.Fatalf("pinned submission: %v", err)
	}
	if err := submit(sub, host, pinned, addr, pw+"x", "postmaster@"+domain); err == nil {
		t.Fatal("a wrong password must be refused")
	}
	got, err := email.FetchSMTPCertSHA256(ctx, sub)
	if err != nil || got != pin {
		t.Fatalf("FetchSMTPCertSHA256 = %q, %v; want the probed pin", got, err)
	}
	cfg := func(p string) email.Config {
		return email.Config{Backend: email.BackendSMTP, SMTP: &email.SMTPConfig{Addr: sub, Host: host, Username: addr, Password: pw,
			From: addr, TLSMode: email.TLSStartTLS, PinSHA256: p}}
	}
	sender, err := email.NewSender(cfg(pin))
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(ctx, "postmaster@"+domain, "system email sender proof", "hello"); err != nil {
		t.Fatalf("pinned sender: %v", err)
	}
	wrong, _ := email.NewSender(cfg(strings.Repeat("0", 64)))
	if err := wrong.Send(ctx, "postmaster@"+domain, "must fail", "x"); err == nil {
		t.Fatal("a wrong pin must refuse the connection")
	}
	t.Logf("submission accepted over STARTTLS with pin %s...", pin[:12])
}

// submit sends one message; an empty rcpt stops after AUTH.
func submit(addr, host string, cfg *tls.Config, user, pass, rcpt string) error {
	cl, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer func() { _ = cl.Close() }()
	if err := cl.StartTLS(cfg); err != nil {
		return err
	}
	if err := cl.Auth(smtp.PlainAuth("", user, pass, host)); err != nil {
		return err
	}
	if rcpt == "" {
		return cl.Quit()
	}
	if err := cl.Mail(user); err != nil { //nolint:gosec // fixed test addresses
		return err
	}
	if err := cl.Rcpt(rcpt); err != nil { //nolint:gosec // fixed test addresses
		return err
	}
	w, err := cl.Data()
	if err != nil {
		return err
	}
	msg := "From: " + user + "\r\nTo: " + rcpt + "\r\nSubject: system email live proof\r\n\r\nhello\r\n"
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return cl.Quit()
}
