package golive

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// ScopeControlPlane labels a probe that ran on this server: it cannot prove
// the wider internet can reach it.
const ScopeControlPlane = "control_plane_host"

// Match says how strongly the HTTPS response was tied to this app.
type Match string

// Match values.
const (
	MatchBody       Match = "body"
	MatchHeaders    Match = "headers"
	MatchUnverified Match = "unverified"
	MatchNone       Match = "none"
)

// Reference is what the app itself answers when asked directly, used to tell
// it apart from another server behind the same address.
type Reference struct {
	Status      int
	ContentType string
	Server      string
	BodyHash    string
}

// SelfCheckInput is one end-to-end check of https://domain.
type SelfCheckInput struct {
	Domain string
	// Addr is the host:port to dial, normally this server's own address.
	Addr      string
	Path      string
	Reference *Reference
	// RootCAs replaces the system roots; tests only.
	RootCAs *x509.CertPool
	Timeout time.Duration
}

// SelfCheckResult is the outcome, with the TLS and HTTP facts behind it.
type SelfCheckResult struct {
	Reached   bool      `json:"reached"`
	TLSValid  bool      `json:"tls_valid"`
	TLSError  string    `json:"tls_error,omitempty"`
	Issuer    string    `json:"issuer,omitempty"`
	NotAfter  time.Time `json:"not_after,omitzero"`
	Status    int       `json:"status,omitempty"`
	Match     Match     `json:"match"`
	Error     string    `json:"error,omitempty"`
	Scope     string    `json:"scope"`
	CheckedAt time.Time `json:"checked_at"`
}

const bodyLimit = 64 << 10

// HashBody fingerprints the first bytes of a response body.
func HashBody(r io.Reader) string {
	h := sha256.New()
	_, _ = io.Copy(h, io.LimitReader(r, bodyLimit))
	return hex.EncodeToString(h.Sum(nil))
}

// SelfCheck dials addr, verifies the certificate chain for domain against the
// system roots, requests path and compares the answer with the reference.
func SelfCheck(ctx context.Context, in SelfCheckInput) SelfCheckResult {
	res := SelfCheckResult{Match: MatchNone, Scope: ScopeControlPlane, CheckedAt: time.Now().UTC()}
	if in.Timeout <= 0 {
		in.Timeout = 5 * time.Second
	}
	path := in.Path
	if path == "" {
		path = "/"
	}
	ctx, cancel := context.WithTimeout(ctx, in.Timeout)
	defer cancel()
	var peer *x509.Certificate
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, in.Addr)
		},
		TLSClientConfig: &tls.Config{ServerName: in.Domain, RootCAs: in.RootCAs, MinVersion: tls.VersionTLS12,
			VerifyConnection: func(cs tls.ConnectionState) error {
				if len(cs.PeerCertificates) > 0 {
					peer = cs.PeerCertificates[0]
				}
				return nil
			}},
		DisableKeepAlives: true,
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+in.Domain+path, nil)
	if err != nil {
		res.Error = fmt.Sprintf("build request: %v", err)
		return res
	}
	req.Header.Set("User-Agent", "go-live-check")
	resp, err := client.Do(req)
	if peer != nil {
		res.Issuer, res.NotAfter = peer.Issuer.String(), peer.NotAfter
	}
	if err != nil {
		res.TLSError, res.Error = classify(err), err.Error()
		res.Reached = res.TLSError != "" || peer != nil
		return res
	}
	defer func() { _ = resp.Body.Close() }()
	res.Reached, res.TLSValid, res.Status = true, true, resp.StatusCode
	res.Match = compare(in.Reference, resp)
	return res
}

func classify(err error) string {
	var unknown x509.UnknownAuthorityError
	var host x509.HostnameError
	var invalid x509.CertificateInvalidError
	switch {
	case errors.As(err, &unknown):
		return "the certificate was not signed by an authority browsers trust"
	case errors.As(err, &host):
		return "the certificate does not cover this domain"
	case errors.As(err, &invalid):
		if invalid.Reason == x509.Expired {
			return "the certificate has expired"
		}
		return "the certificate chain is not valid"
	case strings.Contains(err.Error(), "tls:"):
		return "the TLS handshake failed: " + err.Error()
	}
	return ""
}

func compare(ref *Reference, resp *http.Response) Match {
	if ref == nil {
		return MatchUnverified
	}
	if resp.StatusCode != ref.Status {
		return MatchNone
	}
	if ref.BodyHash != "" && HashBody(resp.Body) == ref.BodyHash {
		return MatchBody
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "" && ct == ref.ContentType && resp.Header.Get("Server") == ref.Server {
		return MatchHeaders
	}
	return MatchNone
}
