package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Writes the same ingress settings the dashboard's domain step saves, so the
// installer needs no API token. The running control plane re-reads the row on
// its next ingress pass.
func runEnableHTTPS(ctx context.Context, args []string, stdout io.Writer, openStore func(context.Context) (*store.DB, error)) error {
	fs := flag.NewFlagSet("enable-https", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	domain := fs.String("domain", "", "hostname that already points at this server")
	email := fs.String("email", "", "ACME account contact address")
	staging := fs.Bool("staging", false, "use Let's Encrypt staging (untrusted certificate, no real rate limits)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse enable-https args: %w", err)
	}
	host := strings.ToLower(strings.TrimSpace(*domain))
	contact := strings.TrimSpace(*email)
	if host == "" || contact == "" {
		return errors.New("usage: enable-https --domain HOST --email ADDRESS [--staging]")
	}
	if !ingress.ACMEIssuable(host) {
		return fmt.Errorf("%s cannot get a public certificate (use a real hostname, not an IP, a bare name or a reserved suffix)", host)
	}
	if err := ingress.ValidateACMEContact(contact); err != nil {
		return err
	}

	db, err := openStore(ctx)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = db.Close() }()

	cur, err := db.GetIngressSettings(ctx)
	if err != nil {
		return fmt.Errorf("load ingress settings: %w", err)
	}
	if cur.TLSTerminatedUpstream {
		return errors.New("HTTPS is handled by your proxy: turn off tls_terminated_upstream before enabling certificates here")
	}
	cur.PrimaryDomain = host
	cur.ACMEEnabled = true
	cur.ACMEEmail = contact
	cur.ACMEDirectoryURL = ""
	if *staging {
		cur.ACMEDirectoryURL = api.ACMEStagingDirectoryURL
	}
	if err := db.UpdateIngressSettings(ctx, cur); err != nil {
		return fmt.Errorf("save ingress settings: %w", err)
	}
	kind := "production"
	if *staging {
		kind = "staging (browsers will not trust it)"
	}
	_, _ = fmt.Fprintf(stdout, "HTTPS enabled for %s with a %s certificate. Issuance runs in the background.\n", host, kind)
	return nil
}
