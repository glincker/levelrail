package main

import (
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/certflow"
	"github.com/glincker/levelrail/internal/store"
)

func TestChooseSetupLink(t *testing.T) {
	base := SetupLinkInput{PublicIP: "203.0.113.7", IngressOwns: true, DashboardPort: "8080", SetupPath: "/login#setup=tok"}
	cases := []struct {
		name      string
		mod       func(*SetupLinkInput)
		wantURL   string
		wantHTTPS bool
		wantInfo  string
	}{
		{"cert ready", func(i *SetupLinkInput) { i.Cert = CertReady }, "https://203-0-113-7.sslip.io/login#setup=tok", true, ""},
		{"issuing", func(i *SetupLinkInput) { i.Cert = CertIssuing }, "http://203.0.113.7:8080/login#setup=tok", false, "still being issued"},
		{"ports not ours", func(i *SetupLinkInput) { i.IngressOwns = false }, "http://203.0.113.7:8080/login#setup=tok", false, "ufw allow 80,443"},
		{"coexist", func(i *SetupLinkInput) { i.Coexist = true; i.IngressOwns = false }, "http://127.0.0.1:8080/login#setup=tok", false, "another proxy owns ports 80 and 443"},
		{"no public ip", func(i *SetupLinkInput) { i.PublicIP = "" }, "http://127.0.0.1:8080/login#setup=tok", false, "no public IPv4"},
		{"no cert yet", func(*SetupLinkInput) {}, "http://203.0.113.7:8080/login#setup=tok", false, "https enable"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := base
			c.mod(&in)
			got := chooseSetupLink(in)
			if got.URL != c.wantURL || got.HTTPS != c.wantHTTPS {
				t.Fatalf("got %q https=%v, want %q https=%v", got.URL, got.HTTPS, c.wantURL, c.wantHTTPS)
			}
			if c.wantInfo != "" && !strings.Contains(got.Reason+" "+got.Command, c.wantInfo) {
				t.Fatalf("reason/command %q %q lack %q", got.Reason, got.Command, c.wantInfo)
			}
			if c.wantHTTPS && (got.Reason != "" || got.Command != "") {
				t.Fatalf("https link carries a reason: %+v", got)
			}
		})
	}
}

func TestIssuingFromLedger(t *testing.T) {
	now := time.Now()
	row := func(kind string, ago time.Duration) store.CertLedgerEntry {
		return store.CertLedgerEntry{Domain: "d", Kind: kind, At: now.Add(-ago)}
	}
	w := 15 * time.Minute
	if !issuingFromLedger([]store.CertLedgerEntry{row(certflow.KindStarted, time.Minute)}, "d", now, w) {
		t.Fatal("recent start should be issuing")
	}
	if issuingFromLedger([]store.CertLedgerEntry{row(certflow.KindStarted, time.Minute), row(certflow.KindIssued, 0)}, "d", now, w) {
		t.Fatal("issued after start is not issuing")
	}
	if issuingFromLedger([]store.CertLedgerEntry{row(certflow.KindStarted, time.Hour)}, "d", now, w) {
		t.Fatal("stale start is not issuing")
	}
}
