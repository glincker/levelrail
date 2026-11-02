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
		{"mixed", []string{"ada.ns.cloudflare.com.", "ns1.example.net."}, ProviderOther},
	}
	for _, c := range cases {
		if got := ProviderOf(c.ns); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}
