package dnsinspect

import "testing"

func TestProviderOf(t *testing.T) {
	cases := []struct {
		name string
		ns   []string
		want string
	}{
		{"none", nil, ProviderUnknown},
		{"cloudflare", []string{"ada.ns.cloudflare.com.", "Bob.NS.Cloudflare.com"}, ProviderCloudflare},
		{"route53", []string{"ns-12.awsdns-01.com.", "ns-900.awsdns-60.net."}, ProviderRoute53},
		{"other", []string{"ns1.example.net."}, ProviderOther},
		{"digitalocean", []string{"ns1.digitalocean.com.", "ns2.digitalocean.com"}, ProviderDigitalOcean},
		{"hetzner other tld", []string{"helium.ns.hetzner.de."}, ProviderOther},
		{"hetzner ok", []string{"hydrogen.ns.hetzner.com.", "ns1.first-ns.de."}, ProviderHetzner},
		{"namecheap", []string{"dns1.registrar-servers.com.", "dns2.registrar-servers.com."}, ProviderNamecheap},
		{"namecheap hosting", []string{"dns1.namecheaphosting.com."}, ProviderNamecheap},
		{"godaddy", []string{"ns01.domaincontrol.com.", "ns02.domaincontrol.com."}, ProviderGoDaddy},
		{"porkbun", []string{"curitiba.ns.porkbun.com.", "fortaleza.ns.porkbun.com."}, ProviderPorkbun},
		{"google cloud dns", []string{"ns-cloud-a1.googledomains.com.", "ns-cloud-a2.googledomains.com."}, ProviderGoogle},
		{"google domains", []string{"ns1.googledomains.com.", "ns2.googledomains.com."}, ProviderGoogle},
		{"azure", []string{"ns1-01.azure-dns.com.", "ns2-01.azure-dns.net.", "ns3-01.azure-dns.org.", "ns4-01.azure-dns.info."}, ProviderAzure},
		{"ns1", []string{"dns1.p01.nsone.net.", "dns2.p01.nsone.net."}, ProviderNS1},
		{"desec", []string{"ns1.desec.io.", "ns2.desec.org."}, ProviderDeSEC},
		{"lookalike digitalocean", []string{"ns1.evil-digitalocean.com.example"}, ProviderOther},
		{"lookalike glued", []string{"ns1.evildigitalocean.com"}, ProviderOther},
		{"lookalike godaddy", []string{"notdomaincontrol.com"}, ProviderOther},
		{"bare apex", []string{"domaincontrol.com"}, ProviderOther},
		{"mixed providers", []string{"ns1.digitalocean.com", "ns01.domaincontrol.com"}, ProviderOther},
		{"mixed", []string{"ada.ns.cloudflare.com.", "ns1.example.net."}, ProviderOther},
	}
	for _, c := range cases {
		if got := ProviderOf(c.ns); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}
