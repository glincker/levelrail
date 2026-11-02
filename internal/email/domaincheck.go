package email

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// Domain check record types and statuses.
const (
	RecordSPF   = "spf"
	RecordDKIM  = "dkim"
	RecordDMARC = "dmarc"

	CheckPass = "pass"
	CheckWarn = "warn"
)

// DNSLookup is the slice of net.Resolver the domain check needs, so tests can fake it.
type DNSLookup interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupCNAME(ctx context.Context, name string) (string, error)
}

// RecordCheck is one DNS record's verdict. Code is a stable key the UI translates; Message is the English text the CLI prints.
type RecordCheck struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Code    string `json:"code"`
	Value   string `json:"value,omitempty"`
	Message string `json:"message"`
}

// DomainCheck is the SPF, DKIM and DMARC verdict for a sending domain.
type DomainCheck struct {
	Domain   string        `json:"domain"`
	Provider string        `json:"provider"`
	Records  []RecordCheck `json:"records"`
	// SPFInclude is the provider's expected SPF include, shown as a hint.
	SPFInclude string `json:"spf_include,omitempty"`
}

var providerSPFInclude = map[Backend]string{
	BackendSES:      "amazonses.com",
	BackendSESSMTP:  "amazonses.com",
	BackendResend:   "amazonses.com",
	BackendPostmark: "spf.mtasv.net",
	BackendSendGrid: "sendgrid.net",
	BackendMailgun:  "mailgun.org",
}

var providerDKIMSelectors = map[Backend][]string{
	BackendResend:   {"resend"},
	BackendSendGrid: {"s1", "s2"},
	BackendMailgun:  {"smtp", "mta", "k1", "krs"},
}

var commonDKIMSelectors = []string{"default", "selector1", "selector2", "google", "k1", "dkim", "mail"}

// DomainOf returns the domain part of an address ("Name <a@b.com>" or "a@b.com").
func DomainOf(from string) string {
	addr := from
	if i := strings.LastIndex(from, "<"); i >= 0 {
		addr = strings.TrimSuffix(from[i+1:], ">")
	}
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		return strings.ToLower(strings.TrimSpace(addr[i+1:]))
	}
	return ""
}

// CheckDomain looks up SPF, DKIM and DMARC for domain and compares them with what provider needs.
func CheckDomain(ctx context.Context, dns DNSLookup, domain string, provider Backend) DomainCheck {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out := DomainCheck{Domain: domain, Provider: string(provider), SPFInclude: providerSPFInclude[provider]}
	out.Records = append(out.Records, checkSPF(ctx, dns, domain, out.SPFInclude), checkDKIM(ctx, dns, domain, provider), checkDMARC(ctx, dns, domain))
	return out
}

func checkSPF(ctx context.Context, dns DNSLookup, domain, include string) RecordCheck {
	rc := RecordCheck{Type: RecordSPF, Name: domain}
	txts, _ := dns.LookupTXT(ctx, domain)
	var spf []string
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=spf1") {
			spf = append(spf, t)
		}
	}
	switch {
	case len(spf) == 0:
		rc.Status, rc.Code, rc.Message = CheckWarn, "spf_missing", "No SPF record found. Publish a TXT record starting with v=spf1 that authorizes your provider."
	case len(spf) > 1:
		rc.Status, rc.Code, rc.Value, rc.Message = CheckWarn, "spf_multiple", spf[0], "More than one SPF record exists. Merge them into one, receivers reject multiple records."
	case strings.Contains(strings.ToLower(spf[0]), "+all"):
		rc.Status, rc.Code, rc.Value, rc.Message = CheckWarn, "spf_permissive", spf[0], "SPF ends in +all, which authorizes every sender. Use ~all or -all."
	case CountSPFLookups(ctx, dns, spf[0], SPFLimitsFromEnv().Depth) > SPFLimitsFromEnv().Lookups:
		rc.Status, rc.Code, rc.Value = CheckWarn, CodeSPFTooManyLookups, spf[0]
		rc.Message = fmt.Sprintf("SPF needs more than %d DNS lookups, so receivers treat it as an error. Remove unused include terms or flatten them.", SPFLimitsFromEnv().Lookups)
	case include != "" && !strings.Contains(strings.ToLower(spf[0]), include):
		rc.Status, rc.Code, rc.Value = CheckWarn, "spf_no_include", spf[0]
		rc.Message = fmt.Sprintf("SPF does not include %s. Add include:%s unless your provider uses its own return-path domain.", include, include)
	default:
		rc.Status, rc.Code, rc.Value, rc.Message = CheckPass, "spf_ok", spf[0], "SPF record found."
	}
	return rc
}

func checkDKIM(ctx context.Context, dns DNSLookup, domain string, provider Backend) RecordCheck {
	rc := RecordCheck{Type: RecordDKIM, Name: "_domainkey." + domain}
	selectors := append(append([]string{}, providerDKIMSelectors[provider]...), commonDKIMSelectors...)
	for _, sel := range selectors {
		name := sel + "._domainkey." + domain
		if txts, err := dns.LookupTXT(ctx, name); err == nil && len(txts) > 0 {
			rc.Name, rc.Status, rc.Code, rc.Value, rc.Message = name, CheckPass, "dkim_found", truncate(strings.Join(txts, ""), 80), "DKIM key found for selector "+sel+"."
			return rc
		}
		if cname, err := dns.LookupCNAME(ctx, name); err == nil && cname != "" && strings.TrimSuffix(cname, ".") != name {
			rc.Name, rc.Status, rc.Code, rc.Value, rc.Message = name, CheckPass, "dkim_found", cname, "DKIM CNAME found for selector "+sel+"."
			return rc
		}
	}
	for _, sel := range selectors {
		if name := sel + "._domainkey." + domain; DKIMProxied(ctx, dns, name) {
			return proxiedDKIMCheck(name)
		}
	}
	rc.Status, rc.Code = CheckWarn, "dkim_not_found"
	rc.Message = "No DKIM record found at common selectors. Add the DKIM records your provider shows on its domain verification page."
	return rc
}

func checkDMARC(ctx context.Context, dns DNSLookup, domain string) RecordCheck {
	name := "_dmarc." + domain
	rc := RecordCheck{Type: RecordDMARC, Name: name}
	txts, _ := dns.LookupTXT(ctx, name)
	for _, t := range txts {
		if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(t)), "V=DMARC1") {
			continue
		}
		rc.Value = t
		policy := ""
		for _, part := range strings.Split(t, ";") {
			kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(kv) == 2 && strings.EqualFold(kv[0], "p") {
				policy = strings.ToLower(strings.TrimSpace(kv[1]))
			}
		}
		if policy == "quarantine" || policy == "reject" {
			rc.Status, rc.Code, rc.Message = CheckPass, "dmarc_ok", "DMARC policy is "+policy+"."
		} else {
			rc.Status, rc.Code, rc.Message = CheckWarn, "dmarc_monitor_only", "DMARC policy is none (monitor only). Move to quarantine or reject once SPF and DKIM pass."
		}
		return rc
	}
	rc.Status, rc.Code, rc.Message = CheckWarn, "dmarc_missing", "No DMARC record found. Publish a TXT record at "+name+" such as v=DMARC1; p=none; rua=mailto:you@"+domain+"."
	return rc
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// CheckSPF is the SPF verdict alone, for callers that grade records one by one.
func CheckSPF(ctx context.Context, dns DNSLookup, domain, include string) RecordCheck {
	return checkSPF(ctx, dns, domain, include)
}

// CheckDMARC is the DMARC verdict alone.
func CheckDMARC(ctx context.Context, dns DNSLookup, domain string) RecordCheck {
	return checkDMARC(ctx, dns, domain)
}

// SystemDNS is the control plane's own resolver, so the check needs no external service.
type SystemDNS struct{}

// LookupTXT resolves TXT records through the system resolver.
func (SystemDNS) LookupTXT(ctx context.Context, name string) ([]string, error) {
	return net.DefaultResolver.LookupTXT(ctx, name)
}

// LookupCNAME resolves a CNAME through the system resolver.
func (SystemDNS) LookupCNAME(ctx context.Context, name string) (string, error) {
	return net.DefaultResolver.LookupCNAME(ctx, name)
}
