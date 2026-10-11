package domaindoctor

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ACME next-action codes, matching the API's acme_failure.action values.
const (
	ACMEActionOpenPort80 = "open_port_80"
	ACMEActionFixDNS     = "fix_dns"
	ACMEActionWait       = "wait_rate_limit"
	ACMEActionFixCAA     = "fix_caa"
	ACMEActionOtherProxy = "other_proxy"
	ACMEActionCheckLogs  = "check_logs"
)

// Stored certificate states, matching GET /api/v1/certificates.
const (
	certStatusExpired      = "expired"
	certStatusExpiringSoon = "expiring_soon"
	certRenewalStalled     = "stalled"
	certSourceCustom       = "custom"
)

func (rn *run) checkApp(context.Context) Check {
	c := Check{ID: IDAppRunning, Title: "App is running and ready", Tier: TierTraffic}
	a := rn.facts.AppState
	switch {
	case rn.facts.App == "":
		c.State, c.Detail = StateSkipped, "no app serves this domain"
	case !a.Known:
		c.State, c.Detail = StateUnavailable, "the app has no status yet"
	case !a.Running:
		c.State = StateFail
		c.Title = "App " + rn.facts.App + " is not running"
		c.Detail = a.Detail
		c.Fix = &Fix{Summary: "Start the app.", Action: &Action{Kind: ActionRequest, Label: "Start app", API: "POST /api/v1/apps/" + rn.facts.App + "/start"}}
	case !a.Ready:
		c.State = StateWarn
		c.Title = "App " + rn.facts.App + " is not ready"
		c.Detail = a.Detail
		c.Fix = &Fix{Summary: "Open the app's status and logs to see why its readiness check fails.", Action: &Action{Kind: ActionLink, Label: "Open app", API: "/apps/" + rn.facts.App}}
	default:
		c.State = StatePass
	}
	return c
}

func (rn *run) checkHeldBack(context.Context) Check {
	c := Check{ID: IDHeldBack, Title: "Domain is not held back", Tier: TierTraffic}
	if !rn.facts.HeldBack {
		c.State = StatePass
		return c
	}
	c.State = StateFail
	c.Title = rn.facts.Domain + " is held back by an import"
	c.Detail = "an app import staged this domain without routing it, so nothing answers for it yet"
	c.Fix = &Fix{Summary: "Finish the import cutover and enable routing for the app.", Action: &Action{Kind: ActionLink, Label: "Open import", API: "/apps/import"}}
	return c
}

func (rn *run) checkPortOwners(context.Context) Check {
	c := Check{ID: IDProxyPorts, Title: "Ports 80 and 443 reach this platform", Tier: TierTraffic}
	if !rn.facts.PortHoldersKnown {
		c.State, c.Detail = StateUnavailable, "container port listing is not available on this control plane"
		return c
	}
	if len(rn.facts.PortHolders) == 0 {
		c.State = StatePass
		return c
	}
	var parts []string
	for _, h := range rn.facts.PortHolders {
		label := h.Container
		if h.Kind != "" {
			label += " (" + h.Kind + ")"
		}
		parts = append(parts, fmt.Sprintf("port %d: %s", h.Port, label))
	}
	c.Detail = "another container publishes " + strings.Join(parts, ", ")
	if rn.facts.TLSTerminatedUpstream {
		c.State = StatePass
		c.Detail += "; it is configured as the proxy in front of this platform"
		return c
	}
	c.State = StateWarn
	c.Title = "Another proxy owns port 80 or 443"
	c.Fix = &Fix{Summary: "Forward this domain from that proxy to this platform, or set it up as the proxy in front.",
		Action: &Action{Kind: ActionRequest, Label: "Show proxy setup", API: "GET /api/v1/system/reverse-proxy?domain=" + rn.facts.Domain}}
	return c
}

func (rn *run) checkStoredCert(context.Context) Check {
	c := Check{ID: IDStoredCert, Title: "Certificate is issued and renewing", Tier: TierCertificate}
	if rn.facts.TLSTerminatedUpstream {
		c.State, c.Detail = StateSkipped, "TLS is terminated by your own proxy"
		return c
	}
	renew := &Action{Kind: ActionRequest, Label: "Renew now", API: "POST " + rn.appDomainAPI("/cert/renew")}
	if rn.facts.App == "" {
		renew = nil
	}
	if f := rn.facts.ACMEFailure; f != nil && f.Action != ACMEActionWait {
		c.State = StateFail
		c.Title = "Let's Encrypt could not issue a certificate"
		if f.Renewal {
			c.Title = "Certificate renewal is failing"
		}
		c.Detail = f.Error
		c.Fix = acmeFix(f.Action, renew)
		return c
	}
	cert := rn.facts.Cert
	if cert == nil {
		if !rn.facts.ACMEEnabled {
			c.State, c.Detail = StateSkipped, "trusted certificates are off; the internal issuer is used"
			return c
		}
		c.State, c.Detail = StateWarn, "no certificate is stored yet"
		if rn.facts.ACMEFailure != nil {
			c.Detail = "Let's Encrypt is rate limiting this domain: " + rn.facts.ACMEFailure.Error
		}
		c.Fix = &Fix{Summary: "Issuance starts once DNS points here and port 80 is open.", Action: renew}
		return c
	}
	days := int(cert.NotAfter.Sub(rn.opts.Now()).Hours() / 24)
	switch {
	case cert.Status == certStatusExpired:
		c.State, c.Title = StateFail, "Certificate has expired"
		c.Detail = "expired " + cert.NotAfter.UTC().Format(time.DateOnly)
		c.Fix = &Fix{Summary: "Renew the certificate.", Action: renew}
	case cert.Renewal == certRenewalStalled:
		c.State, c.Title = StateFail, "Certificate renewal is stalled"
		c.Detail = fmt.Sprintf("expires in %d days and has not renewed", days)
		c.Fix = &Fix{Summary: "Renew the certificate and check that port 80 reaches this server.", Action: renew}
	case cert.Status == certStatusExpiringSoon || days < rn.opts.ExpiryWarnDays:
		c.State, c.Tier, c.Title = StateWarn, TierSoon, fmt.Sprintf("Certificate expires in %d days", days)
		c.Detail = "issued by " + cert.Issuer
		if cert.Source == certSourceCustom {
			c.Fix = &Fix{Summary: "Upload a renewed certificate; your own certificates do not renew automatically.",
				Action: &Action{Kind: ActionRequest, Label: "Replace certificate", API: "PUT " + rn.appDomainAPI("/tls-cert")}}
		} else {
			c.Fix = &Fix{Summary: "Renewal should happen on its own; renew now if it does not.", Action: renew}
		}
	default:
		c.State = StatePass
		c.Detail = fmt.Sprintf("issued by %s, expires in %d days", cert.Issuer, days)
	}
	return c
}

func acmeFix(action string, renew *Action) *Fix {
	switch action {
	case ACMEActionOpenPort80:
		return &Fix{Summary: "Let's Encrypt could not reach port 80. Open port 80 on this server and in any cloud firewall, then renew.", Action: renew}
	case ACMEActionFixDNS:
		return &Fix{Summary: "Point the domain's DNS at this server, then renew.", Action: renew}
	case ACMEActionFixCAA:
		return &Fix{Summary: "Allow letsencrypt.org in the domain's CAA records, then renew.", Action: renew}
	case ACMEActionOtherProxy:
		return &Fix{Summary: "Another proxy answered the challenge. Route /.well-known/acme-challenge/ to this platform or let that proxy terminate TLS.",
			Action: &Action{Kind: ActionLink, Label: "Show proxy setup", API: "/proxy"}}
	default:
		return &Fix{Summary: "Check the control plane logs for the CA's full error, then renew.", Action: renew}
	}
}
