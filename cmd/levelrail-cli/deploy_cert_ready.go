package main

import (
	"context"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	envDeployCertWait     = "APP_DEPLOY_CERT_WAIT"
	defaultDeployCertWait = "2m"
	urlWaitPollInterval   = 2 * time.Second
)

// primaryDomain returns the host of a URL built from the app's first domain, or "" for a fallback URL.
func primaryDomain(app appResource) string {
	if len(app.Domains) == 0 {
		return ""
	}
	return app.Domains[0]
}

// domainCertState reads the per-domain certificate state (pending, issued, failed) that
// "domains cert" uses. "" means the state could not be read, so no claim is made either way.
func domainCertState(ctx context.Context, c *Client, domain string) string {
	state, _ := domainCertInfo(ctx, c, domain)
	return state
}

// domainCertInfo is domainCertState plus the issuer class (internal, letsencrypt, ...).
func domainCertInfo(ctx context.Context, c *Client, domain string) (state, kind string) {
	st, err := c.DomainCertStatus(ctx, domain)
	if err != nil {
		return "", ""
	}
	return st.State, st.Kind
}

// waitForDomainCert polls until the certificate leaves pending or the window closes.
func waitForDomainCert(ctx context.Context, c *Client, domain string, window time.Duration) string {
	ctx, cancel := context.WithTimeout(ctx, window)
	defer cancel()
	deadline := time.Now().Add(window)
	for {
		state := domainCertState(ctx, c, domain)
		if state != certPending || !time.Now().Add(urlWaitPollInterval).Before(deadline) {
			return state
		}
		select {
		case <-ctx.Done():
			return state
		case <-time.After(urlWaitPollInterval):
		}
	}
}

// certNote is the stderr line that qualifies a live URL whose certificate is not ready.
func certNote(url, state string) string {
	switch state {
	case certPending:
		return "live on " + url + " (certificate is being issued, usually under a minute; the URL may not answer until then)"
	case certFailed:
		return "live on " + url + " (certificate issuance failed, check \"domains certificates\")"
	}
	return ""
}

func deployURLWait(lookupEnv func(string) (string, bool)) (time.Duration, error) {
	v, ok := lookupEnv(envDeployCertWait)
	if !ok || strings.TrimSpace(v) == "" {
		v = defaultDeployCertWait
	}
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil || d <= 0 {
		return 0, newValidationError("%s must be a positive duration, got %q", envDeployCertWait, v)
	}
	return d, nil
}

const certKindInternal = "internal"

// publicURL returns the address clients really use: the app's primary URL plus the public HTTPS
// port when ingress runs on a non-default one (the stored domain has no port).
func publicURL(ctx context.Context, c *Client, app appResource) string {
	raw := appPrimaryURL(app)
	u, err := url.Parse(raw)
	if raw == "" || err != nil || u.Port() != "" || len(app.Domains) == 0 {
		return raw
	}
	ing, err := c.GetIngressSettings(ctx)
	if err != nil || ing.PublicHTTPSPort <= 0 || ing.PublicHTTPSPort == 443 {
		return raw
	}
	u.Host = net.JoinHostPort(u.Hostname(), strconv.Itoa(ing.PublicHTTPSPort))
	return u.String()
}

// fallbackCertKind reads who issued the certificate behind an automatic (no domain) URL,
// bounded so a slow status route never holds up the deploy summary.
func fallbackCertKind(ctx context.Context, c *Client, rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 2*urlWaitPollInterval)
	defer cancel()
	st, err := c.DomainCertStatus(ctx, u.Hostname())
	if err != nil {
		return ""
	}
	return st.Kind
}

// internalCANote warns that browsers will not trust an internal-CA certificate.
func internalCANote(rawURL, kind string) string {
	if kind != certKindInternal {
		return ""
	}
	return rawURL + " uses a certificate from the internal CA: trust that CA on your devices or expect a browser warning"
}
