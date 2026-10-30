package mailrecords

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/email"
)

// Check statuses.
const (
	StatusPass      = "pass"
	StatusWarn      = "warn"
	StatusFail      = "fail"
	StatusNotTested = "not_tested"
)

// Resolver is the slice of net.Resolver the checks need, so tests can fake it.
type Resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupCNAME(ctx context.Context, name string) (string, error)
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
	LookupAddr(ctx context.Context, addr string) ([]string, error)
	LookupSRV(ctx context.Context, service, proto, name string) (string, []*net.SRV, error)
}

// Expect carries what the operator intends, so the check can compare.
type Expect struct {
	MailHost     string `json:"mail_host,omitempty"`
	DKIMSelector string `json:"dkim_selector,omitempty"`
}

// Item is one check verdict.
type Item struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Value   string `json:"value,omitempty"`
}

// Report is the full live check of a domain.
type Report struct {
	Domain string `json:"domain"`
	Items  []Item `json:"items"`
}

func trimDot(s string) string { return strings.TrimSuffix(strings.ToLower(s), ".") }

// Check resolves the live DNS for domain and grades each mail record.
func Check(ctx context.Context, r Resolver, domain string, exp Expect) (Report, error) {
	if err := ValidHost(domain); err != nil {
		return Report{}, fmt.Errorf("domain: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	rep := Report{Domain: domain}
	mx, host := checkMX(ctx, r, domain, exp.MailHost)
	rep.Items = append(rep.Items, mx)
	rep.Items = append(rep.Items, fromEmail(email.CheckSPF(ctx, r, domain, ""), true))
	rep.Items = append(rep.Items, checkDKIM(ctx, r, domain, exp.DKIMSelector))
	rep.Items = append(rep.Items, fromEmail(email.CheckDMARC(ctx, r, domain), true))
	rep.Items = append(rep.Items,
		txtPresence(ctx, r, "mta_sts", "_mta-sts."+domain, "v=stsv1", "MTA-STS record found; also serve the policy file over HTTPS.", "No MTA-STS record. Optional, but it forces encrypted delivery."),
		txtPresence(ctx, r, "tls_rpt", "_smtp._tls."+domain, "v=tlsrptv1", "TLS-RPT record found.", "No TLS-RPT record. Optional, it only adds delivery failure reports."),
		checkAutoconfig(ctx, r, domain))
	rep.Items = append(rep.Items, checkFCrDNS(ctx, r, host)...)
	rep.Items = append(rep.Items, Item{ID: "port25", Status: StatusNotTested, Message: "Outbound port 25 is not probed here. Many cloud hosts block it by default; ask the host to open it before sending."})
	return rep, nil
}

func checkMX(ctx context.Context, r Resolver, domain, want string) (Item, string) {
	mx, err := r.LookupMX(ctx, domain)
	if err != nil || len(mx) == 0 {
		return Item{ID: "mx", Status: StatusFail, Message: "No MX record resolves, so no one can send mail to this domain."}, trimDot(want)
	}
	var hosts []string
	for _, m := range mx {
		hosts = append(hosts, fmt.Sprintf("%d %s", m.Pref, trimDot(m.Host)))
	}
	it := Item{ID: "mx", Status: StatusPass, Message: "MX resolves.", Value: strings.Join(hosts, " | ")}
	host := trimDot(want)
	if host == "" {
		host = trimDot(mx[0].Host)
	}
	if want != "" {
		found := false
		for _, m := range mx {
			found = found || trimDot(m.Host) == host
		}
		if !found {
			it.Status, it.Message = StatusWarn, "MX resolves but does not include "+host+"."
		}
	}
	return it, host
}

func fromEmail(rc email.RecordCheck, failOnMissing bool) Item {
	it := Item{ID: rc.Type, Status: StatusPass, Message: rc.Message, Value: rc.Value}
	if rc.Status != email.CheckPass {
		it.Status = StatusWarn
		switch rc.Code {
		case "spf_missing", "spf_multiple", "spf_permissive", "dmarc_missing":
			if failOnMissing {
				it.Status = StatusFail
			}
		}
	}
	return it
}

func checkDKIM(ctx context.Context, r Resolver, domain, sel string) Item {
	if sel == "" {
		return Item{ID: "dkim", Status: StatusWarn, Message: "No DKIM selector given, so the key was not checked. Pass the selector you published."}
	}
	name := sel + "._domainkey." + domain
	txts, _ := r.LookupTXT(ctx, name)
	val := strings.Join(txts, "")
	if val == "" {
		if c, err := r.LookupCNAME(ctx, name); err == nil && trimDot(c) != name && c != "" {
			return Item{ID: "dkim", Status: StatusPass, Message: "DKIM selector " + sel + " is delegated by CNAME.", Value: trimDot(c)}
		}
		return Item{ID: "dkim", Status: StatusFail, Message: "No DKIM record at " + name + "."}
	}
	var p string
	for _, part := range strings.Split(val, ";") {
		if kv := strings.SplitN(strings.TrimSpace(part), "=", 2); len(kv) == 2 && kv[0] == "p" {
			p = strings.TrimSpace(kv[1])
		}
	}
	if p == "" {
		return Item{ID: "dkim", Status: StatusFail, Message: "DKIM key at " + name + " is empty (revoked).", Value: truncate(val)}
	}
	if der, err := base64.StdEncoding.DecodeString(p); err == nil {
		if pub, err := x509.ParsePKIXPublicKey(der); err == nil {
			if k, ok := pub.(*rsa.PublicKey); ok && k.N.BitLen() < 2048 {
				st := StatusWarn
				if k.N.BitLen() < 1024 {
					st = StatusFail
				}
				return Item{ID: "dkim", Status: st, Message: fmt.Sprintf("DKIM RSA key is %d bits; use 2048.", k.N.BitLen()), Value: truncate(val)}
			}
		}
	}
	return Item{ID: "dkim", Status: StatusPass, Message: "DKIM key found for selector " + sel + ".", Value: truncate(val)}
}

func truncate(s string) string {
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}

func txtPresence(ctx context.Context, r Resolver, id, name, prefix, ok, missing string) Item {
	txts, _ := r.LookupTXT(ctx, name)
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), prefix) {
			return Item{ID: id, Status: StatusPass, Message: ok, Value: t}
		}
	}
	return Item{ID: id, Status: StatusWarn, Message: missing}
}

func checkAutoconfig(ctx context.Context, r Resolver, domain string) Item {
	if c, err := r.LookupCNAME(ctx, "autoconfig."+domain); err == nil && c != "" && trimDot(c) != "autoconfig."+domain {
		return Item{ID: "autoconfig", Status: StatusPass, Message: "autoconfig CNAME found.", Value: trimDot(c)}
	}
	if _, srv, err := r.LookupSRV(ctx, "submission", "tcp", domain); err == nil && len(srv) > 0 {
		return Item{ID: "autoconfig", Status: StatusPass, Message: "Client SRV records (RFC 6186) found.", Value: trimDot(srv[0].Target)}
	}
	return Item{ID: "autoconfig", Status: StatusWarn, Message: "No autoconfig CNAME or submission SRV record. Mail apps will need manual server settings."}
}

// checkFCrDNS verifies each IP of host has a PTR that resolves back to the IP.
func checkFCrDNS(ctx context.Context, r Resolver, host string) []Item {
	if host == "" {
		return []Item{{ID: "fcrdns", Status: StatusWarn, Message: "No mail host known, so reverse DNS was not checked."}}
	}
	ips, err := r.LookupHost(ctx, host)
	if err != nil || len(ips) == 0 {
		return []Item{{ID: "fcrdns", Status: StatusFail, Message: host + " has no A or AAAA record."}}
	}
	var out []Item
	for _, ip := range ips {
		it := Item{ID: "fcrdns", Value: ip}
		names, _ := r.LookupAddr(ctx, ip)
		switch {
		case len(names) == 0:
			it.Status, it.Message = StatusFail, "No PTR for "+ip+". Set reverse DNS at the IP owner (the hosting provider); receivers often reject mail without it."
		case ptrMatches(ctx, r, names, host, ip):
			it.Status, it.Message = StatusPass, "PTR of "+ip+" resolves back to "+host+"."
		default:
			it.Status, it.Message = StatusFail, fmt.Sprintf("PTR of %s is %s, which does not match %s.", ip, trimDot(names[0]), host)
		}
		out = append(out, it)
	}
	return out
}

func ptrMatches(ctx context.Context, r Resolver, names []string, host, ip string) bool {
	for _, n := range names {
		if trimDot(n) != host {
			continue
		}
		addrs, _ := r.LookupHost(ctx, host)
		for _, a := range addrs {
			if a == ip {
				return true
			}
		}
	}
	return false
}
