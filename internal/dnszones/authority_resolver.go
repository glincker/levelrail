package dnszones

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
)

// NetAuthorityResolver answers AuthorityResolver from a net.Resolver. The standard
// library exposes no TTL, so it does not implement TTLResolver and the TTL risk stays off.
type NetAuthorityResolver struct {
	Resolver *net.Resolver
}

func (n NetAuthorityResolver) res() *net.Resolver {
	if n.Resolver != nil {
		return n.Resolver
	}
	return net.DefaultResolver
}

// LookupNS finds the name servers for name, walking up to the parent zone when a
// subdomain has no NS records of its own. It never goes above a two-label name.
func (n NetAuthorityResolver) LookupNS(ctx context.Context, name string) ([]string, error) {
	cur := NormalizeDomain(name)
	for {
		recs, err := n.res().LookupNS(ctx, cur)
		if err != nil {
			var dnsErr *net.DNSError
			if !errors.As(err, &dnsErr) || !dnsErr.IsNotFound {
				return nil, err
			}
		}
		if len(recs) > 0 {
			out := make([]string, 0, len(recs))
			for _, r := range recs {
				out = append(out, r.Host)
			}
			return out, nil
		}
		i := strings.IndexByte(cur, '.')
		if i < 0 || strings.Count(cur, ".") < 2 {
			return nil, nil
		}
		cur = cur[i+1:]
	}
}

// LookupIP returns the A and AAAA answers for name.
func (n NetAuthorityResolver) LookupIP(ctx context.Context, name string) ([]netip.Addr, error) {
	return n.res().LookupNetIP(ctx, "ip", name)
}

// LookupCNAME returns the canonical name, which equals name when there is no CNAME.
func (n NetAuthorityResolver) LookupCNAME(ctx context.Context, name string) (string, error) {
	return n.res().LookupCNAME(ctx, name)
}
