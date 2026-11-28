package ddns

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"golang.org/x/net/publicsuffix"

	"github.com/glincker/levelrail/internal/dnszones"
	"github.com/glincker/levelrail/internal/secretmask"
)

// zoneProvider keeps A or AAAA records current through a token DNS provider's record API.
type zoneProvider struct {
	p     dnszones.Provider
	hosts []string
	ttl   int
}

// Lowest TTLs each service accepts on a standard account.
const (
	porkbunMinTTL = 600
	desecMinTTL   = 3600
)

// NewPorkbun is the preset that updates hostnames through Porkbun's record API. credential is "apikey:secretapikey".
func NewPorkbun(domains, credential string) (Provider, error) {
	p, err := dnszones.NewPorkbun(credential)
	if err != nil {
		return nil, err
	}
	secretmask.Add(credential)
	return newZoneProvider(p, domains, porkbunMinTTL)
}

// NewDeSEC is the preset that updates hostnames through deSEC's rrset API with an API token.
func NewDeSEC(domains, token string) (Provider, error) {
	secretmask.Add(token)
	return newZoneProvider(dnszones.NewDeSEC(token), domains, desecMinTTL)
}

func newZoneProvider(p dnszones.Provider, domains string, ttl int) (*zoneProvider, error) {
	hosts := splitList(domains)
	if len(hosts) == 0 {
		return nil, errors.New("ddns: list the full hostnames to keep updated")
	}
	for _, h := range hosts {
		if _, _, err := splitHost(h); err != nil {
			return nil, err
		}
	}
	return &zoneProvider{p: p, hosts: hosts, ttl: ttl}, nil
}

// splitHost returns the registrable zone and the record name inside it.
func splitHost(host string) (zone, name string, err error) {
	h := dnszones.NormalizeDomain(host)
	zone, err = publicsuffix.EffectiveTLDPlusOne(h)
	if err != nil {
		return "", "", fmt.Errorf("ddns: %q is not a full hostname", host)
	}
	return zone, dnszones.RelativeName(h, zone), nil
}

// Update writes ip to every hostname. Errors never carry the credential.
func (z *zoneProvider) Update(ctx context.Context, ip string) error {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return errors.New("ddns: the detected address is not valid")
	}
	typ := "A"
	if addr.Is6() && !addr.Is4In6() {
		typ = "AAAA"
	}
	var failed []string
	for _, h := range z.hosts {
		zone, name, _ := splitHost(h)
		rs := dnszones.RecordSet{Name: name, Type: typ, TTL: z.ttl, Values: []string{addr.Unmap().String()}, Routing: dnszones.RoutingSimple}
		if err := z.p.UpsertRecordSet(ctx, dnszones.Zone{ID: zone, Name: zone}, rs); err != nil {
			failed = append(failed, h+": "+secretmask.Mask(err.Error()))
		}
	}
	if len(failed) > 0 {
		return errors.New(strings.Join(failed, "; "))
	}
	return nil
}
