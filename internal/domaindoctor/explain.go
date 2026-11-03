package domaindoctor

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnsinspect"
)

// maxEvidenceLine keeps a pasted CA or resolver error from flooding the list.
const maxEvidenceLine = 240

// guideCtx is what a steps builder may use to word a fix for this domain.
type guideCtx struct {
	domain   string
	app      string
	provider string
	rec      *Record
	server   string
}

// entry is the fixed plain-language text for one check id. It is a lookup
// table, so the wording is the same on every run and works offline.
type entry struct {
	fail  string
	warn  string
	label string
	// fixID names an existing fixit fix for the same problem, empty when none.
	fixID string
	link  func(g guideCtx) string
	steps func(g guideCtx) []string
}

func (g guideCtx) recordText() string {
	if g.rec != nil {
		return fmt.Sprintf("%s record named %s with the value %s", g.rec.Type, g.rec.Name, g.rec.Value)
	}
	return "record for " + g.domain + " that points at this server's public address"
}

// providerWhere says where the DNS settings live, without needing a terminal.
func providerWhere(p string) string {
	switch p {
	case dnsinspect.ProviderCloudflare:
		return "Sign in to Cloudflare, choose your domain, then open DNS and Records."
	case dnsinspect.ProviderRoute53:
		return "Sign in to the AWS console, open Route 53, choose Hosted zones, then open the zone for your domain."
	case dnsinspect.ProviderGoDaddy:
		return "Sign in to GoDaddy, open My Products, then choose DNS next to your domain."
	case dnsinspect.ProviderNamecheap:
		return "Sign in to Namecheap, open Domain List, choose Manage next to your domain, then open Advanced DNS."
	case dnsinspect.ProviderDigitalOcean:
		return "Sign in to DigitalOcean, open Networking, choose Domains, then open your domain."
	case dnsinspect.ProviderPorkbun:
		return "Sign in to Porkbun, open Domain Management, then choose DNS next to your domain."
	case dnsinspect.ProviderGoogle:
		return "Sign in to Google Domains or Squarespace Domains, choose your domain, then open DNS."
	case dnsinspect.ProviderHetzner:
		return "Sign in to the Hetzner DNS console and open the zone for your domain."
	case dnsinspect.ProviderAzure:
		return "Sign in to the Azure portal, open DNS zones, then open the zone for your domain."
	}
	return "Open the DNS settings at the company that runs your domain's name servers, usually where you bought the domain."
}

// dnsSteps wraps one record change in the provider's own navigation.
func dnsSteps(g guideCtx, change ...string) []string {
	out := []string{providerWhere(g.provider)}
	out = append(out, change...)
	return append(out, "Save the change, wait a few minutes, then press Re-check.")
}

func stepsOf(s ...string) func(guideCtx) []string {
	return func(guideCtx) []string { return s }
}

func appLink(g guideCtx) string {
	if g.app == "" {
		return ""
	}
	return "/apps/" + url.PathEscape(g.app)
}

var explainTable = map[string]entry{
	IDDNSResolves: {
		fail:  "The internet has no address on record for this domain, so nobody can reach it yet.",
		warn:  "Some DNS servers already know the new address and others still remember the old one.",
		label: "Add the DNS record",
		steps: func(g guideCtx) []string {
			return dnsSteps(g, "Add a "+g.recordText()+".", "If a record with the same name already exists, change it instead of adding a second one.")
		},
	},
	IDDNSPointsHere: {
		fail:  "The domain points at a different server than this one, so visitors land somewhere else.",
		warn:  "The domain points at another address, which is expected only when your own proxy or CDN forwards traffic here.",
		label: "Point the domain at this server",
		steps: func(g guideCtx) []string {
			return dnsSteps(g, "Find the record named "+g.domain+" and change its value to "+orText(g.server, "this server's public address")+".",
				"If a proxy or CDN in front is meant to forward here, no change is needed; check that it forwards this domain to this server.")
		},
	},
	IDDNSAAAA: {
		fail:  "The domain has an IPv6 address on record that is not this server, so visitors who use IPv6 reach the wrong place.",
		label: "Fix or remove the IPv6 record",
		steps: func(g guideCtx) []string {
			return dnsSteps(g, "Find the AAAA record named "+g.domain+".", "Delete it, or change its value to this server's IPv6 address if the server has one.")
		},
	},
	IDDNSApexCNAME: {
		warn:  "The bare domain has an alias record, which many DNS providers reject and which can break email for the domain.",
		label: "Use an address record at the bare domain",
		steps: func(g guideCtx) []string {
			return dnsSteps(g, "Replace the alias (CNAME) record on the bare domain with an A record pointing at this server.",
				"If your provider offers ALIAS or flattened CNAME records, that also works.")
		},
	},
	IDCAA: {
		fail:  "A CAA record limits which companies may issue certificates for this domain, and Let's Encrypt is not on the list.",
		label: "Allow Let's Encrypt",
		steps: func(g guideCtx) []string {
			return dnsSteps(g, "Add a CAA record with flag 0, tag issue and value letsencrypt.org, or edit the existing CAA record to include it.")
		},
	},
	IDAppRunning: {
		fail:  "The app that should answer for this domain is stopped or failing.",
		warn:  "The app is running but its readiness check has not passed, so traffic may not reach it.",
		label: "Start the app and check it",
		link:  appLink,
		steps: stepsOf("Press Start app if the button is offered, or open the app page.", "If it stops again, open the app's logs on that page to read the error.", "Press Re-check when the app shows as healthy."),
	},
	IDHeldBack: {
		fail:  "An app import saved this domain but has not turned routing on, so nothing answers for it yet.",
		label: "Finish the import",
		link:  func(guideCtx) string { return "/apps/import" },
		steps: stepsOf("Open the import page.", "Finish the cutover step for this app and turn on routing.", "Press Re-check."),
	},
	IDProxyPorts: {
		warn:  "Another program on this machine is using the public web ports, so visitors may reach it instead of this platform.",
		label: "Forward the domain from the other proxy",
		steps: stepsOf("Open the proxy setup page and read the settings shown for this domain.", "In the other proxy, add a route that forwards this domain to this platform.",
			"Or stop the other program so this platform can use the web ports, then press Re-check."),
	},
	IDPort80: {
		fail:  "Nothing answers on the plain web port (80), and Let's Encrypt needs it to prove you own the domain.",
		warn:  "The plain web port (80) did not answer from here, which may be a firewall or just this server's own network.",
		label: "Open port 80",
		fixID: "host.firewall.open_web_ports",
		steps: stepsOf("Open the firewall page of your hosting provider (the security group or firewall for this server).", "Allow incoming traffic on port 80 from anywhere.",
			"If the server has its own firewall, allow port 80 there as well, then press Re-check."),
	},
	IDTLS: {
		fail:  "The secure connection to the domain does not present a certificate that matches it, so browsers show a warning or refuse to connect.",
		warn:  "The certificate is close to expiring or is not fully trusted by browsers.",
		label: "Get a valid certificate",
		link:  appLink,
		steps: stepsOf("Open the app's domain settings and press Renew certificate, or turn on trusted certificates under Settings, then Ingress.",
			"If another proxy answers on the secure port, add this domain to it or let this platform own the port.", "Press Re-check."),
	},
	IDHTTPStatus: {
		fail:  "The site answers, but with an error, so the app behind the domain is down or crashing.",
		warn:  "The site answers with not found or could not be fetched, so the route for this domain may be missing.",
		label: "Check the app behind the domain",
		link:  appLink,
		steps: stepsOf("Open the app page and make sure it is running.", "Read the app's logs for the error it reports.", "Check that the app listens on the port set for it, then press Re-check."),
	},
	IDRedirects: {
		fail:  "The domain redirects in a loop, so the page never loads.",
		warn:  "Visitors who use plain http are not sent to the secure address.",
		label: "Fix the redirects",
		link:  appLink,
		steps: stepsOf("Open the domain settings for the app.", "Turn on Force HTTPS so plain http visitors are sent to https.",
			"If a proxy or CDN sits in front, mark TLS as handled upstream, or turn off the redirect in either the proxy or this platform, then press Re-check."),
	},
	IDHSTS: {
		fail:  "Browsers are told to insist on a secure connection for this domain, but the certificate is not trusted, so visitors would be locked out.",
		label: "Get a trusted certificate first",
		steps: stepsOf("Fix the certificate problem listed above so a trusted certificate is served.", "Leave the strict-security header as it is until the certificate works.", "Press Re-check."),
	},
	IDStoredCert: {
		fail:  "A certificate could not be issued or renewed for this domain.",
		warn:  "The certificate is not issued yet or is getting close to its end date.",
		label: "Renew the certificate",
		fixID: "certs.renew",
		link:  appLink,
		steps: stepsOf("Make sure the domain points at this server and port 80 is open, using the checks above.", "Open the app's domain settings and press Renew now.",
			"If it fails again, the reason shown there names the exact step to fix. Press Re-check afterwards."),
	},
	IDDNSProxied: {
		warn:  "The domain sits behind Cloudflare's proxy, so this server's address cannot be checked from outside and certificates may not validate.",
		label: "Check the Cloudflare proxy setting",
		steps: func(g guideCtx) []string {
			return []string{providerWhere(dnsinspect.ProviderCloudflare),
				"Find the record named " + g.domain + " and make sure it points at " + orText(g.server, "this server's address") + ".",
				"Either set SSL/TLS to Full (strict), or switch the record to DNS only (grey cloud) so certificates validate directly.", "Save, then press Re-check."}
		},
	},
	IDDNSWildcard: {
		warn:  "A wildcard certificate needs a DNS account the platform can use to prove ownership, and none is connected.",
		label: "Connect a DNS provider",
		link:  func(guideCtx) string { return "/domains" },
		steps: stepsOf("Open Domains and choose Connect DNS provider.", "Add a Cloudflare or Route 53 token that may edit DNS records.", "Press Re-check."),
	},
	IDTimeout: {
		warn:  "The checks ran out of time before every one finished.",
		label: "Run the checks again",
		steps: stepsOf("Press Re-check to run the checks again.", "If it keeps timing out, a slow DNS server or a blocked connection is the usual cause; check the network of this server."),
	},
}

func orText(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// providerFor classifies the zone from the name servers that answered, and
// falls back to the connected DNS account.
func (rn *run) providerFor() string {
	var ns []string
	for _, a := range rn.answers {
		if name, ok := strings.CutPrefix(a.Resolver, "authoritative "); ok && a.Authoritative {
			ns = append(ns, name)
		}
	}
	p := dnsinspect.ProviderOf(ns)
	if (p == dnsinspect.ProviderUnknown || p == dnsinspect.ProviderOther) && rn.facts.DNSProvider != "" {
		return rn.facts.DNSProvider
	}
	return p
}

// explain fills the plain-words cause, evidence and fix steps for a check
// that is not passing. Passing and skipped checks stay quiet.
func (rn *run) explain(c *Check) {
	if c.State == StatePass || c.State == StateSkipped {
		return
	}
	e, ok := explainTable[c.ID]
	if !ok {
		return
	}
	c.Cause = e.fail
	if (c.State != StateFail && e.warn != "") || e.fail == "" {
		c.Cause = e.warn
	}
	c.Evidence = rn.evidence(c)

	g := guideCtx{domain: rn.facts.Domain, app: rn.facts.App, provider: rn.providerFor(), server: firstOr(rn.facts.ServerIPs, "")}
	if recs := PlanRecords(rn.facts.Domain, rn.facts.ServerIPs, rn.facts.ServerHost); len(recs) > 0 {
		g.rec = &recs[0]
	}
	if c.Fix == nil {
		c.Fix = &Fix{Summary: e.label}
	}
	c.Fix.ID, c.Fix.Label, c.Fix.Steps = e.fixID, e.label, e.steps(g)
	if e.link != nil {
		c.Fix.Link = e.link(g)
	}
}

func clip(s string) string {
	if len(s) > maxEvidenceLine {
		return s[:maxEvidenceLine] + "..."
	}
	return s
}

func listOr(items []string, none string) string {
	if len(items) == 0 {
		return none
	}
	return strings.Join(items, ", ")
}

// evidence lists the observed facts behind a check. Only public data is
// used: records, addresses, name servers and certificate details.
func (rn *run) evidence(c *Check) []string {
	var out []string
	add := func(format string, a ...any) { out = append(out, clip(fmt.Sprintf(format, a...))) }
	dnsFacts := func() {
		for _, a := range rn.answers {
			switch addrs := append(append([]string{}, a.IPv4...), a.IPv6...); {
			case len(addrs) > 0:
				add("%s answered %s", a.Resolver, strings.Join(addrs, ", "))
			case a.Error != "":
				add("%s failed: %s", a.Resolver, a.Error)
			default:
				add("%s had no answer", a.Resolver)
			}
		}
		add("Server address expected: %s", listOr(rn.facts.ServerIPs, "not known"))
	}
	switch c.ID {
	case IDDNSResolves, IDDNSPointsHere, IDDNSAAAA:
		dnsFacts()
	case IDAppRunning:
		add("App status: %s", orText(rn.facts.AppState.Detail, "unknown"))
	case IDProxyPorts:
		for _, h := range rn.facts.PortHolders {
			add("Port %d is held by %s (%s)", h.Port, h.Container, h.Kind)
		}
	case IDTLS:
		if pc := rn.tls; pc != nil {
			add("Certificate is for %s, issued by %s", listOr(pc.DNSNames, pc.Subject), pc.Issuer)
			if !pc.NotAfter.IsZero() {
				add("Certificate ends on %s", pc.NotAfter.UTC().Format(time.DateOnly))
			}
		}
	case IDStoredCert:
		if cert := rn.facts.Cert; cert != nil {
			add("Certificate issued by %s, ends on %s", cert.Issuer, cert.NotAfter.UTC().Format(time.DateOnly))
		}
		if f := rn.facts.ACMEFailure; f != nil {
			add("The certificate authority said: %s", f.Error)
		}
	}
	if c.Detail != "" {
		add("%s", c.Detail)
	}
	return out
}
