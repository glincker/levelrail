package api

import (
	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/mail/demo"
	"github.com/GLINCKER/levelrail/internal/mailrecords"
)

// mailDemoProvider is the fake detection demo mode serves: a Cloudflare host connected to the demo zone.
func mailDemoProvider(domain string, z *demo.Zone) dnsProviderResponse {
	domain = dnszones.NormalizeDomain(domain)
	if domain == "" {
		domain = demo.DefaultDomain
	}
	apex, err := mailrecords.RegistrableApex(domain)
	if err != nil {
		apex = demo.DefaultDomain
	}
	return dnsProviderResponse{
		Detection: mailrecords.Detection{
			Domain: domain, Apex: apex, Zone: apex,
			Nameservers:   []string{"ada.ns.cloudflare.com", "bob.ns.cloudflare.com"},
			Provider:      mailrecords.HostCloudflare,
			DomainConnect: mailrecords.DomainConnect{Supported: true, Host: "api.cloudflare.com", Path: "/client/v4/dns/domainconnect"},
		},
		Connected: true, Connectable: true, ZoneID: z.ZoneFor(apex).ID, CanIssueCertificates: true,
		Hint: "Demo mode: a fake Cloudflare zone. Nothing is written to a real DNS provider.",
	}
}
