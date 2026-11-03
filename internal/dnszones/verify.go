package dnszones

import (
	"context"
	"errors"
	"net"
	"strings"

	"golang.org/x/oauth2"
)

// Access outcomes reported by VerifyAccess.
const (
	AccessOK          = "ok"
	AccessInvalid     = "invalid"
	AccessDenied      = "denied"
	AccessUnreachable = "unreachable"
	AccessError       = "error"
)

// AccessReport is what a credential can reach, from read-only calls only.
type AccessReport struct {
	Outcome      string
	ZonesVisible int
	// Missing names what the product needs and the credential lacks.
	Missing []string
}

// Permission names the product needs per provider, shown when a probe is denied.
const (
	cfZoneRead        = "Zone:Zone:Read"
	cfDNSEdit         = "Zone:DNS:Edit"
	r53ListZones      = "route53:ListHostedZones"
	r53ListRecordSets = "route53:ListResourceRecordSets"
	gcdDNSReader      = "roles/dns.reader"
)

// VerifyAccess lists zones, then the records of the first zone, with the
// cheapest read calls. Edit rights cannot be proven without a write.
func VerifyAccess(ctx context.Context, p Provider) AccessReport {
	zones, err := p.ListZones(ctx)
	if err != nil {
		return failedAccess(p.Name(), err, zoneReadNeed(p.Name()))
	}
	rep := AccessReport{Outcome: AccessOK, ZonesVisible: len(zones)}
	if len(zones) == 0 {
		return rep
	}
	if _, err := p.ListRecordSets(ctx, zones[0]); err != nil {
		out := failedAccess(p.Name(), err, recordReadNeed(p.Name()))
		out.ZonesVisible = len(zones)
		return out
	}
	return rep
}

func zoneReadNeed(provider string) string {
	switch provider {
	case ProviderCloudflare:
		return cfZoneRead
	case ProviderRoute53:
		return r53ListZones
	default:
		return gcdDNSReader
	}
}

func recordReadNeed(provider string) string {
	switch provider {
	case ProviderCloudflare:
		return cfDNSEdit
	case ProviderRoute53:
		return r53ListRecordSets
	default:
		return gcdDNSReader
	}
}

func failedAccess(_ string, err error, need string) AccessReport {
	switch classifyAccessError(err) {
	case AccessInvalid:
		return AccessReport{Outcome: AccessInvalid}
	case AccessDenied:
		return AccessReport{Outcome: AccessDenied, Missing: []string{need}}
	case AccessUnreachable:
		return AccessReport{Outcome: AccessUnreachable}
	}
	return AccessReport{Outcome: AccessError}
}

func classifyAccessError(err error) string {
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.As(err, &ne) {
		return AccessUnreachable
	}
	var cf *CloudflareError
	if errors.As(err, &cf) {
		return statusAccess(cf.Status, cloudflareInvalid(cf))
	}
	var g *GoogleCloudError
	if errors.As(err, &g) {
		return statusAccess(g.Status, false)
	}
	var rerr *oauth2.RetrieveError
	if errors.As(err, &rerr) {
		return AccessInvalid
	}
	var coded interface{ ErrorCode() string }
	if errors.As(err, &coded) {
		switch coded.ErrorCode() {
		case "InvalidClientTokenId", "UnrecognizedClientException", "SignatureDoesNotMatch", "ExpiredToken", "InvalidSignatureException":
			return AccessInvalid
		case "AccessDenied", "AccessDeniedException", "UnauthorizedOperation":
			return AccessDenied
		}
	}
	return AccessError
}

func statusAccess(status int, invalid bool) string {
	switch {
	case invalid || status == 401:
		return AccessInvalid
	case status == 403:
		return AccessDenied
	}
	return AccessError
}

// cloudflareInvalid spots the error codes Cloudflare uses for a malformed
// or unknown token, which arrive as 400 or 403 rather than 401.
func cloudflareInvalid(e *CloudflareError) bool {
	for _, m := range e.Messages {
		for _, code := range []string{"6003 ", "6111 ", "9103 ", "1000 "} {
			if strings.HasPrefix(m, code) {
				return true
			}
		}
	}
	return false
}
