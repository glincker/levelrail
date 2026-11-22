package main

import (
	"context"
	"crypto/tls"
	"net"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/certflow"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// CertState is what the host knows about the certificate for the sslip.io
// name of its public address.
type CertState int

const (
	CertNone CertState = iota
	CertIssuing
	CertReady
)

// SetupLinkInput is everything chooseSetupLink needs, so the choice stays pure.
type SetupLinkInput struct {
	PublicIP      string
	IngressOwns   bool // Levelrail ingress holds ports 80 and 443
	Coexist       bool // another proxy owns 80 and 443
	Cert          CertState
	DashboardPort string
	SetupPath     string // "/login#setup=<token>" or empty
}

// SetupLink is the best dashboard URL plus why it is not HTTPS and the one
// command that fixes it.
type SetupLink struct {
	URL     string
	HTTPS   bool
	Reason  string
	Command string
}

// sslipName returns the dashed sslip.io host for a plain IPv4, or "".
func sslipName(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil || !a.Is4() {
		return ""
	}
	return strings.ReplaceAll(a.String(), ".", "-") + ".sslip.io"
}

func chooseSetupLink(in SetupLinkInput) SetupLink {
	name := sslipName(in.PublicIP)
	if name == "" {
		return SetupLink{
			URL:     "http://127.0.0.1:" + in.DashboardPort + in.SetupPath,
			Reason:  "no public IPv4 address was found, so there is no free HTTPS name for this server",
			Command: "ssh -L " + in.DashboardPort + ":127.0.0.1:" + in.DashboardPort + " <user>@<server-ip>",
		}
	}
	plain := "http://" + in.PublicIP + ":" + in.DashboardPort + in.SetupPath
	switch {
	case in.Coexist:
		return SetupLink{
			URL:     "http://127.0.0.1:" + in.DashboardPort + in.SetupPath,
			Reason:  "HTTPS was not issued because another proxy owns ports 80 and 443; terminate TLS there and point it at Levelrail ingress",
			Command: "ssh -L " + in.DashboardPort + ":127.0.0.1:" + in.DashboardPort + " <user>@" + in.PublicIP,
		}
	case in.Cert == CertReady:
		return SetupLink{URL: "https://" + name + in.SetupPath, HTTPS: true}
	case in.Cert == CertIssuing:
		return SetupLink{
			URL:     plain,
			Reason:  "the HTTPS certificate for " + name + " is still being issued",
			Command: "levelrail-cli settings ingress https status",
		}
	case !in.IngressOwns:
		return SetupLink{
			URL:     plain,
			Reason:  "ports 80 and 443 are not served by Levelrail ingress, so a certificate cannot be issued",
			Command: "sudo ufw allow 80,443/tcp && sudo systemctl restart levelrail",
		}
	}
	return SetupLink{
		URL:     plain,
		Reason:  "no HTTPS certificate exists yet for " + name,
		Command: "levelrail-cli settings ingress https enable --email you@example.com",
	}
}

// ingressOwnsWebPorts is true when the configured ingress addresses are the
// standard 80 and 443 (coexist mode moves them).
func ingressOwnsWebPorts() bool {
	return portOf(os.Getenv("APP_INGRESS_HTTP_ADDR"), "80") == "80" &&
		portOf(os.Getenv("APP_INGRESS_HTTPS_ADDR"), "443") == "443"
}

func portOf(addr, def string) string {
	if addr == "" {
		return def
	}
	if _, p, err := net.SplitHostPort(addr); err == nil && p != "" {
		return p
	}
	return def
}

// issuingFromLedger is true when domain has a recent started entry with no
// issued or failed entry after it.
func issuingFromLedger(rows []store.CertLedgerEntry, domain string, now time.Time, window time.Duration) bool {
	var started, done time.Time
	for _, r := range rows {
		if r.Domain != domain {
			continue
		}
		switch r.Kind {
		case certflow.KindStarted:
			if r.At.After(started) {
				started = r.At
			}
		case certflow.KindIssued, certflow.KindFailed:
			if r.At.After(done) {
				done = r.At
			}
		}
	}
	return !started.IsZero() && now.Sub(started) <= window && !done.After(started)
}

// probeCertState checks the local listener for a certificate that browsers
// would accept for name, then the stored ledger for a pending issuance.
func probeCertState(ctx context.Context, db *store.DB, name string) CertState {
	if name == "" {
		return CertNone
	}
	pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	d := tls.Dialer{Config: &tls.Config{ServerName: name, MinVersion: tls.VersionTLS12}}
	if conn, err := d.DialContext(pctx, "tcp", net.JoinHostPort("127.0.0.1", portOf(os.Getenv("APP_INGRESS_HTTPS_ADDR"), "443"))); err == nil {
		_ = conn.Close()
		return CertReady
	}
	now := time.Now()
	window := time.Duration(clampMinutes(runtimesettings.New(db, os.LookupEnv).Int(ctx, runtimesettings.KeySetupHTTPSIssuingWindow))) * time.Minute
	if rows, err := db.LoadCertLedger(ctx, now, certflow.LedgerKeep); err == nil && issuingFromLedger(rows, name, now, window) {
		return CertIssuing
	}
	return CertNone
}

func clampMinutes(n int) int { return min(max(n, 1), 240) }
