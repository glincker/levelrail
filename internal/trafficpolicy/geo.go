package trafficpolicy

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"
)

// Geo modes, actions and unknown-country handling.
const (
	GeoAllow = "allow"
	GeoDeny  = "deny"

	GeoActionBlock     = "block"
	GeoActionRedirect  = "redirect"
	GeoActionErrorPage = "error_page"

	GeoUnknownAllow = "allow"
	GeoUnknownBlock = "block"
)

// Geo is a domain's country access rule.
type Geo struct {
	Mode        string   `json:"mode"`
	Countries   []string `json:"countries"`
	Action      string   `json:"action"`
	RedirectURL string   `json:"redirect_url,omitempty"`
	StatusCode  int      `json:"status_code,omitempty"`
	Body        string   `json:"body,omitempty"`
	// Exempt addresses and ranges are never blocked by country.
	Exempt []string `json:"exempt,omitempty"`
	// Unknown decides requests whose country cannot be determined.
	Unknown string `json:"unknown,omitempty"`
}

// UnknownMode returns the effective unknown-country handling (allow by default).
func (g Geo) UnknownMode() string {
	if g.Unknown == "" {
		return GeoUnknownAllow
	}
	return g.Unknown
}

// BlockStatus returns the status code for the block and error page actions.
func (g Geo) BlockStatus() int {
	if g.StatusCode == 0 {
		return 403
	}
	return g.StatusCode
}

// Validate checks the rule.
func (g *Geo) Validate(l Limits) error {
	col := &collector{kind: KindGeo}
	if g.Mode != GeoAllow && g.Mode != GeoDeny {
		col.add("mode", "must be allow or deny")
	}
	if len(g.Countries) == 0 {
		col.add("countries", "list at least one country code")
	}
	if len(g.Countries) > len(isoCountries) {
		col.add("countries", "too many countries")
	}
	for i, c := range g.Countries {
		if !IsCountryCode(c) {
			col.add(fmt.Sprintf("countries[%d]", i), "%q is not an ISO 3166-1 alpha-2 country code", c)
		}
	}
	switch g.Action {
	case GeoActionBlock:
	case GeoActionRedirect:
		u, err := url.Parse(g.RedirectURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || !SafeHeaderValue(g.RedirectURL) {
			col.add("redirect_url", "must be an absolute http(s) URL")
		}
	case GeoActionErrorPage:
		if strings.TrimSpace(g.Body) == "" {
			col.add("body", "is required for a custom error page")
		}
	default:
		col.add("action", "must be block, redirect or error_page")
	}
	if len(g.Body) > l.MaxBodyBytes {
		col.add("body", "must be at most %d bytes", l.MaxBodyBytes)
	}
	if g.StatusCode != 0 && g.StatusCode != 403 && g.StatusCode != 451 && g.StatusCode != 404 {
		col.add("status_code", "must be 403, 404 or 451")
	}
	if len(g.Exempt) > l.MaxExemptEntries {
		col.add("exempt", "at most %d entries", l.MaxExemptEntries)
	}
	for i, e := range g.Exempt {
		if _, err := ParseIPOrPrefix(e); err != nil {
			col.add(fmt.Sprintf("exempt[%d]", i), "must be an IP address or CIDR range")
		}
	}
	if g.Unknown != "" && g.Unknown != GeoUnknownAllow && g.Unknown != GeoUnknownBlock {
		col.add("unknown", "must be allow or block")
	}
	return col.err()
}

// ParseIPOrPrefix accepts "203.0.113.7" or "203.0.113.0/24".
func ParseIPOrPrefix(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("trafficpolicy: parse prefix: %w", err)
		}
		return p.Masked(), nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("trafficpolicy: parse address: %w", err)
	}
	a = a.Unmap()
	return netip.PrefixFrom(a, a.BitLen()), nil
}

// NeverGeoBlocked reports addresses geo rules never apply to: private,
// loopback, link-local and unspecified addresses have no country.
func NeverGeoBlocked(a netip.Addr) bool {
	a = a.Unmap()
	return !a.IsValid() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsUnspecified() ||
		netip.MustParsePrefix("100.64.0.0/10").Contains(a)
}

// GeoDecision is the outcome of evaluating a Geo rule for one request.
type GeoDecision struct {
	Blocked bool
	Reason  string
}

// Decide evaluates g for a client address and its resolved country ("" when
// unknown). exempt is g.Exempt parsed once by the caller.
func (g Geo) Decide(client netip.Addr, country string, exempt []netip.Prefix) GeoDecision {
	if NeverGeoBlocked(client) {
		return GeoDecision{Reason: "private or loopback address"}
	}
	for _, p := range exempt {
		if p.Contains(client.Unmap()) {
			return GeoDecision{Reason: "exempt address"}
		}
	}
	country = strings.ToUpper(strings.TrimSpace(country))
	if !IsCountryCode(country) {
		return GeoDecision{Blocked: g.UnknownMode() == GeoUnknownBlock, Reason: "unknown country"}
	}
	listed := false
	for _, c := range g.Countries {
		if strings.EqualFold(c, country) {
			listed = true
			break
		}
	}
	if g.Mode == GeoAllow {
		if listed {
			return GeoDecision{Reason: country + " is allowed"}
		}
		return GeoDecision{Blocked: true, Reason: country + " is not on the allow list"}
	}
	if listed {
		return GeoDecision{Blocked: true, Reason: country + " is on the deny list"}
	}
	return GeoDecision{Reason: country + " is not on the deny list"}
}

// IsCountryCode reports whether c is an assigned ISO 3166-1 alpha-2 code.
func IsCountryCode(c string) bool {
	if len(c) != 2 {
		return false
	}
	return isoCountries[strings.ToUpper(c)]
}

var isoCountries = func() map[string]bool {
	const codes = "AD AE AF AG AI AL AM AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BR BS BT BV BW BY BZ " +
		"CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR " +
		"GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP " +
		"KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT " +
		"MU MV MW MX MY MZ NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RE RO RS RU RW " +
		"SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR SS ST SV SX SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG " +
		"UM US UY UZ VA VC VE VG VI VN VU WF WS XK YE YT ZA ZM ZW"
	m := make(map[string]bool, 260)
	for _, c := range strings.Fields(codes) {
		m[c] = true
	}
	return m
}()
