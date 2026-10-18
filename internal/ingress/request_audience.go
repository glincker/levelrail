package ingress

import (
	"net/http"
	"strings"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// requestReferrerHost is the Referer host, empty when it equals the served
// host: internal navigation is not a traffic source.
func requestReferrerHost(r *http.Request) string {
	host := telemetry.ReferrerHost(r.Header.Get("Referer"))
	if host == "" || strings.EqualFold(host, hostWithoutPort(r.Host)) {
		return ""
	}
	return host
}

func hostWithoutPort(h string) string {
	if c := strings.LastIndex(h, ":"); c >= 0 && !strings.Contains(h[c:], "]") {
		return h[:c]
	}
	return h
}

// requestCountry is empty unless a geo source (trusted header or MMDB) is
// active, so geo stays optional and costs nothing when off.
func requestCountry(r *http.Request) string {
	res := DefaultGeoResolver()
	if !res.Status().Active {
		return ""
	}
	trusted, _ := caddyhttp.GetVar(r.Context(), caddyhttp.TrustedProxyVarKey).(bool)
	country, _ := res.Country(requestClientAddr(r), r.Header.Get(res.HeaderName()), trusted)
	return country
}
