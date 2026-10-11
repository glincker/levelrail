package trafficpolicy

import (
	"strings"
	"testing"
)

func fieldsOf(t *testing.T, err error) []string {
	t.Helper()
	if err == nil {
		return nil
	}
	ve, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("error %v is not a ValidationError", err)
	}
	out := make([]string, 0, len(ve.Fields))
	for _, f := range ve.Fields {
		out = append(out, f.Field)
	}
	return out
}

func wantField(t *testing.T, err error, field string) {
	t.Helper()
	if field == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	for _, f := range fieldsOf(t, err) {
		if f == field {
			return
		}
	}
	t.Fatalf("want error on %q, got %v", field, err)
}

func TestHeadersValidate(t *testing.T) {
	l := DefaultLimits()
	ctxTLS := Context{Domain: "app.example.com", TLSReal: true}
	tests := []struct {
		name  string
		h     Headers
		ctx   Context
		field string
	}{
		{"ok response set", Headers{Rules: []HeaderRule{{Side: SideResponse, Op: OpSet, Name: "X-Frame-Options", Value: "DENY"}}}, ctxTLS, ""},
		{"ok request remove", Headers{Rules: []HeaderRule{{Side: SideRequest, Op: OpRemove, Name: "X-Debug"}}}, ctxTLS, ""},
		{"bad side", Headers{Rules: []HeaderRule{{Side: "both", Op: OpSet, Name: "X-A", Value: "1"}}}, ctxTLS, "rules[0].side"},
		{"bad op", Headers{Rules: []HeaderRule{{Side: SideRequest, Op: "replace", Name: "X-A", Value: "1"}}}, ctxTLS, "rules[0].op"},
		{"set needs value", Headers{Rules: []HeaderRule{{Side: SideRequest, Op: OpSet, Name: "X-A"}}}, ctxTLS, "rules[0].value"},
		{"remove takes no value", Headers{Rules: []HeaderRule{{Side: SideRequest, Op: OpRemove, Name: "X-A", Value: "x"}}}, ctxTLS, "rules[0].value"},
		{"invalid name", Headers{Rules: []HeaderRule{{Side: SideRequest, Op: OpSet, Name: "X A", Value: "1"}}}, ctxTLS, "rules[0].name"},
		{"crlf injection", Headers{Rules: []HeaderRule{{Side: SideResponse, Op: OpSet, Name: "X-A", Value: "1\r\nSet-Cookie: a=b"}}}, ctxTLS, "rules[0].value"},
		{"nul byte", Headers{Rules: []HeaderRule{{Side: SideResponse, Op: OpSet, Name: "X-A", Value: "a\x00b"}}}, ctxTLS, "rules[0].value"},
		{"host forbidden", Headers{Rules: []HeaderRule{{Side: SideRequest, Op: OpSet, Name: "host", Value: "evil"}}}, ctxTLS, "rules[0].name"},
		{"content-length forbidden", Headers{Rules: []HeaderRule{{Side: SideResponse, Op: OpRemove, Name: "Content-Length"}}}, ctxTLS, "rules[0].name"},
		{"transfer-encoding forbidden", Headers{Rules: []HeaderRule{{Side: SideRequest, Op: OpAdd, Name: "Transfer-Encoding", Value: "chunked"}}}, ctxTLS, "rules[0].name"},
		{"connection forbidden", Headers{Rules: []HeaderRule{{Side: SideRequest, Op: OpSet, Name: "Connection", Value: "close"}}}, ctxTLS, "rules[0].name"},
		{"upgrade forbidden", Headers{Rules: []HeaderRule{{Side: SideRequest, Op: OpRemove, Name: "Upgrade"}}}, ctxTLS, "rules[0].name"},
		{"x-forwarded forbidden", Headers{Rules: []HeaderRule{{Side: SideRequest, Op: OpSet, Name: "X-Forwarded-For", Value: "1.2.3.4"}}}, ctxTLS, "rules[0].name"},
		{"x-cache reserved", Headers{Rules: []HeaderRule{{Side: SideResponse, Op: OpSet, Name: "X-Cache", Value: "HIT"}}}, ctxTLS, "rules[0].name"},
		{"value too long", Headers{Rules: []HeaderRule{{Side: SideResponse, Op: OpSet, Name: "X-A", Value: strings.Repeat("a", l.MaxHeaderValueLen+1)}}}, ctxTLS, "rules[0].value"},
		{"hsts without real tls", Headers{Security: &SecurityPreset{HSTS: true}}, Context{Domain: "a.example.com"}, "security.hsts"},
		{"hsts with tls", Headers{Security: &SecurityPreset{HSTS: true, HSTSMaxAge: 600}}, ctxTLS, ""},
		{"bad frame options", Headers{Security: &SecurityPreset{FrameOptions: "ALLOW"}}, ctxTLS, "security.frame_options"},
		{"bad referrer", Headers{Security: &SecurityPreset{ReferrerPolicy: "everything"}}, ctxTLS, "security.referrer_policy"},
		{"cors ok", Headers{CORS: &CORSPreset{Origins: []string{"https://a.example.com"}, Methods: []string{"GET"}, Credentials: true}}, ctxTLS, ""},
		{"cors star with credentials", Headers{CORS: &CORSPreset{Origins: []string{"*"}, Credentials: true}}, ctxTLS, "cors.origins[0]"},
		{"cors origin with path", Headers{CORS: &CORSPreset{Origins: []string{"https://a.example.com/x"}}}, ctxTLS, "cors.origins[0]"},
		{"cors origin with quote", Headers{CORS: &CORSPreset{Origins: []string{"https://a.example.com'"}}}, ctxTLS, "cors.origins[0]"},
		{"cors empty", Headers{CORS: &CORSPreset{}}, ctxTLS, "cors.origins"},
		{"forwarded prefix", Headers{ForwardedPrefix: "api"}, ctxTLS, "forwarded_prefix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantField(t, tt.h.Validate(l, tt.ctx), tt.field)
		})
	}
	t.Run("rule cap", func(t *testing.T) {
		l := DefaultLimits()
		l.MaxHeaderRules = 1
		h := Headers{Rules: []HeaderRule{{Side: SideRequest, Op: OpRemove, Name: "A"}, {Side: SideRequest, Op: OpRemove, Name: "B"}}}
		wantField(t, h.Validate(l, ctxTLS), "rules")
	})
}

func TestForwardersValidate(t *testing.T) {
	l := DefaultLimits()
	c := Context{Domain: "app.example.com", AppExists: func(n string) bool { return n == "api" }}
	prefix := func(p string) Match { return Match{Kind: MatchPrefix, Path: p} }
	tests := []struct {
		name  string
		f     Forwarder
		field string
	}{
		{"app ok", Forwarder{Match: prefix("/api"), Action: ActionApp, App: "api", StripPrefix: true}, ""},
		{"unknown app", Forwarder{Match: prefix("/api"), Action: ActionApp, App: "nope"}, "rules[0].app"},
		{"url ok", Forwarder{Match: prefix("/docs"), Action: ActionURL, URL: "https://docs.example.org/base"}, ""},
		{"url to self loops", Forwarder{Match: prefix("/x"), Action: ActionURL, URL: "https://APP.example.com"}, "rules[0].url"},
		{"url with credentials", Forwarder{Match: prefix("/x"), Action: ActionURL, URL: "https://u:p@docs.example.org"}, "rules[0].url"}, //nolint:gosec // fake credentials under test
		{"url relative", Forwarder{Match: prefix("/x"), Action: ActionURL, URL: "/elsewhere"}, "rules[0].url"},
		{"url with placeholder", Forwarder{Match: prefix("/x"), Action: ActionURL, URL: "https://x.org/{env.SECRET}"}, "rules[0].url"},
		{"redirect ok", Forwarder{Match: prefix("/old"), Action: ActionRedirect, URL: "https://app.example.com/new", RedirectStatus: 308}, ""},
		{"redirect loop", Forwarder{Match: prefix("/"), Action: ActionRedirect, URL: "https://app.example.com/new"}, "rules[0].url"},
		{"redirect bad status", Forwarder{Match: prefix("/old"), Action: ActionRedirect, URL: "https://x.org", RedirectStatus: 303}, "rules[0].redirect_status"},
		{"regex ok", Forwarder{Match: Match{Kind: MatchRegex, Path: `^/v[0-9]+/`}, Action: ActionApp, App: "api"}, ""},
		{"regex invalid", Forwarder{Match: Match{Kind: MatchRegex, Path: `(`}, Action: ActionApp, App: "api"}, "rules[0].match.path"},
		{"regex lookahead unsupported", Forwarder{Match: Match{Kind: MatchRegex, Path: `(?=a)`}, Action: ActionApp, App: "api"}, "rules[0].match.path"},
		{"regex too long", Forwarder{Match: Match{Kind: MatchRegex, Path: strings.Repeat("a", 300)}, Action: ActionApp, App: "api"}, "rules[0].match.path"},
		{"prefix with wildcard", Forwarder{Match: prefix("/api/*"), Action: ActionApp, App: "api"}, "rules[0].match.path"},
		{"prefix without slash", Forwarder{Match: prefix("api"), Action: ActionApp, App: "api"}, "rules[0].match.path"},
		{"bad method", Forwarder{Match: Match{Kind: MatchPrefix, Path: "/a", Methods: []string{"FETCH"}}, Action: ActionApp, App: "api"}, "rules[0].match.methods[0]"},
		{"strip on exact", Forwarder{Match: Match{Kind: MatchExact, Path: "/a"}, Action: ActionApp, App: "api", StripPrefix: true}, "rules[0].strip_prefix"},
		{"rewrite placeholder", Forwarder{Match: prefix("/a"), Action: ActionApp, App: "api", RewritePrefix: "/{x}"}, "rules[0].rewrite_prefix"},
		{"custom host empty", Forwarder{Match: prefix("/a"), Action: ActionApp, App: "api", Host: HostCustom}, "rules[0].host_value"},
		{"timeout too long", Forwarder{Match: prefix("/a"), Action: ActionApp, App: "api", TimeoutSeconds: 100000}, "rules[0].timeout_seconds"},
		{"bad action", Forwarder{Match: prefix("/a"), Action: "proxy"}, "rules[0].action"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fw := Forwarders{Rules: []Forwarder{tt.f}}
			wantField(t, fw.Validate(l, c), tt.field)
		})
	}
}

func TestGeoValidateAndDecide(t *testing.T) {
	l := DefaultLimits()
	tests := []struct {
		name  string
		g     Geo
		field string
	}{
		{"ok", Geo{Mode: GeoDeny, Countries: []string{"RU", "kp"}, Action: GeoActionBlock}, ""},
		{"bad code", Geo{Mode: GeoDeny, Countries: []string{"ZZ"}, Action: GeoActionBlock}, "countries[0]"},
		{"bad mode", Geo{Mode: "block", Countries: []string{"US"}, Action: GeoActionBlock}, "mode"},
		{"redirect needs url", Geo{Mode: GeoAllow, Countries: []string{"US"}, Action: GeoActionRedirect}, "redirect_url"},
		{"page needs body", Geo{Mode: GeoAllow, Countries: []string{"US"}, Action: GeoActionErrorPage}, "body"},
		{"bad exempt", Geo{Mode: GeoAllow, Countries: []string{"US"}, Action: GeoActionBlock, Exempt: []string{"10.0.0.0/33"}}, "exempt[0]"},
		{"bad status", Geo{Mode: GeoAllow, Countries: []string{"US"}, Action: GeoActionBlock, StatusCode: 500}, "status_code"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { wantField(t, tt.g.Validate(l), tt.field) })
	}
}

func TestCacheValidate(t *testing.T) {
	l := DefaultLimits()
	rule := func(r CacheRule) Cache { return Cache{Enabled: true, Rules: []CacheRule{r}} }
	tests := []struct {
		name  string
		c     Cache
		field string
	}{
		{"ok", rule(CacheRule{Match: Match{Kind: MatchPrefix, Path: "/assets"}, TTLSeconds: 60}), ""},
		{"enabled without rules", Cache{Enabled: true}, "rules"},
		{"post not cacheable", rule(CacheRule{Match: Match{Kind: MatchPrefix, Path: "/", Methods: []string{"POST"}}, TTLSeconds: 60}), "rules[0].match.methods[0]"},
		{"override needs ttl", rule(CacheRule{Match: Match{Kind: MatchPrefix, Path: "/"}, OverrideUpstream: true}), "rules[0].ttl_seconds"},
		{"bad status", rule(CacheRule{Match: Match{Kind: MatchPrefix, Path: "/"}, TTLSeconds: 1, StatusCodes: []int{500}}), "rules[0].status_codes[0]"},
		{"vary cookie refused", rule(CacheRule{Match: Match{Kind: MatchPrefix, Path: "/"}, TTLSeconds: 1, Vary: []string{"cookie"}}), "rules[0].vary[0]"},
		{"ttl beyond cap", rule(CacheRule{Match: Match{Kind: MatchPrefix, Path: "/"}, TTLSeconds: 10 * 24 * 3600}), "rules[0].ttl_seconds"},
		{"object too big", Cache{MaxObjectBytes: 1 << 40}, "max_object_bytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { wantField(t, tt.c.Validate(l), tt.field) })
	}
}

func TestRedirectsValidate(t *testing.T) {
	wantField(t, (&Redirects{ForceHTTPS: true, ForceHTTPSStatus: 302}).Validate(), "force_https_status")
	wantField(t, (&Redirects{TrailingSlash: "maybe"}).Validate(), "trailing_slash")
	wantField(t, (&Redirects{ForceHTTPS: true, TrailingSlash: SlashAdd}).Validate(), "")
}

func TestLimitsFromEnv(t *testing.T) {
	env := map[string]string{EnvMaxHeaderRules: "3", EnvForwardTimeout: "2s", EnvMaxForwarders: "-1", EnvCacheMaxObject: "nope"}
	l := LimitsFromEnv(func(k string) string { return env[k] })
	d := DefaultLimits()
	if l.MaxHeaderRules != 3 || l.ForwardTimeout.String() != "2s" || l.MaxForwarders != d.MaxForwarders || l.CacheMaxObject != d.CacheMaxObject {
		t.Fatalf("LimitsFromEnv() = %+v", l)
	}
}

func TestPolicyDecode(t *testing.T) {
	var p Policy
	if err := p.Decode(KindGeo, []byte(`{"mode":"deny","countries":["RU"],"action":"block"}`)); err != nil || p.Geo == nil || p.Geo.Mode != GeoDeny {
		t.Fatalf("Decode(geo) = %v, %+v", err, p.Geo)
	}
	if err := p.Decode("nope", []byte(`{}`)); err == nil {
		t.Fatal("Decode(unknown kind) succeeded")
	}
	if err := p.Decode(KindCache, []byte(`{`)); err == nil {
		t.Fatal("Decode(bad json) succeeded")
	}
	if !IsKind(KindHeaders) || IsKind("other") {
		t.Fatal("IsKind wrong")
	}
}
