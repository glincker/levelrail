package platformimport

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

const fakeLabels = `traefik.enable=true
traefik.http.routers.web.rule=Host(` + "`app.example.test`" + `)
traefik.http.middlewares.gate.basicauth.users=fakeuser:$apr1$fakefake$notarealhash
traefik.http.middlewares.tohttps.redirectscheme.scheme=https
traefik.http.middlewares.old.redirectregex.regex=^https?://app.example.test/old/(.*)
traefik.http.middlewares.old.redirectregex.replacement=https://app.example.test/new
traefik.http.middlewares.old.redirectregex.permanent=true
traefik.http.middlewares.api.stripprefix.prefixes=/api
traefik.http.middlewares.hdr.headers.customresponseheaders.X-Frame-Options=DENY
traefik.http.middlewares.hdr.headers.customrequestheaders.Authorization=Bearer fake-token-value-123
traefik.http.middlewares.rl.ratelimit.average=100
some.other.label=fake-hidden-value`

func TestTranslateIngressCoolifyLabels(t *testing.T) {
	p := TranslateIngress(IngressSource{Labels: fakeLabels, Redirect: "non-www", App: "web", Domain: "app.example.test"}, func(string) string { return "" })
	if !p.BasicAuth || !p.ForceHTTPS || p.Canonical != trafficpolicy.PresetWWWToApex {
		t.Fatalf("auth %v https %v canonical %q", p.BasicAuth, p.ForceHTTPS, p.Canonical)
	}
	var redirect, strip bool
	for _, f := range p.Forwarders {
		redirect = redirect || (f.Action == trafficpolicy.ActionRedirect && f.Match.Path == "^/old/(.*)" && f.RedirectStatus == 308)
		strip = strip || (f.StripPrefix && f.App == "web" && f.Match.Path == "/api")
	}
	if !redirect || !strip {
		t.Fatalf("forwarders %+v", p.Forwarders)
	}
	if len(p.Headers) != 1 || p.Headers[0].Name != "X-Frame-Options" {
		t.Fatalf("headers %+v", p.Headers)
	}
	out, _ := json.Marshal(p)
	body := string(out) + strings.Join(func() []string {
		var s []string
		for _, n := range ingressNotes(p) {
			s = append(s, n.Reason, n.Manual)
		}
		return s
	}(), " ")
	for _, secret := range []string{"notarealhash", "fakeuser", "fake-token-value-123", "fake-hidden-value"} {
		if strings.Contains(body, secret) {
			t.Fatalf("plan leaks %q: %s", secret, body)
		}
	}
	for _, want := range []string{"ratelimit", "some.other.label", "Authorization"} {
		if !strings.Contains(body, want) {
			t.Fatalf("manual items miss %q: %s", want, body)
		}
	}
}

func TestTranslateIngressHostileLabels(t *testing.T) {
	huge := strings.Repeat("a", 5000)
	labels := strings.Join([]string{
		"traefik.http.middlewares.js.redirectregex.regex=^/x",
		"traefik.http.middlewares.js.redirectregex.replacement=javascript:alert(1)",
		"traefik.http.middlewares.data.redirectregex.regex=^/y",
		"traefik.http.middlewares.data.redirectregex.replacement=data:text/html,hi",
		"traefik.http.middlewares.bad.redirectregex.regex=^/(unclosed",
		"traefik.http.middlewares.bad.redirectregex.replacement=https://ok.example.test/",
		"traefik.http.middlewares.big.redirectregex.regex=^/" + huge,
		"traefik.http.middlewares.big.redirectregex.replacement=https://ok.example.test/",
		"traefik.http.middlewares.cap.redirectregex.regex=^/(.*)",
		"traefik.http.middlewares.cap.redirectregex.replacement=https://ok.example.test/${1}",
		"traefik.http.middlewares.rep.redirectregex.regex=^/((a{100}){100}){100}",
		"traefik.http.middlewares.rep.redirectregex.replacement=https://ok.example.test/",
		"traefik.http.middlewares.crlf.headers.customresponseheaders.X-Test=ok\rSet-Cookie: x",
		"traefik.http.middlewares.sp.stripprefix.prefixes=no-slash",
	}, "\n")
	enc := base64.StdEncoding.EncodeToString([]byte(labels))
	p := TranslateIngress(IngressSource{Labels: enc, App: "web", Domain: "app.example.test"}, func(string) string { return "" })
	if len(p.Forwarders) != 0 || len(p.Headers) != 0 {
		t.Fatalf("hostile input translated: %+v %+v", p.Forwarders, p.Headers)
	}
	if len(p.Manual) < 7 {
		t.Fatalf("want a manual item per hostile rule, got %+v", p.Manual)
	}
	for _, n := range p.Manual {
		if strings.ContainsAny(n.Reason, "\r\n") || len(n.Reason) > 1024 || strings.Contains(n.Reason, huge) {
			t.Fatalf("unsafe manual reason %q", n.Reason)
		}
	}
	tooBig := TranslateIngress(IngressSource{Labels: "a=" + strings.Repeat("b", 200)}, func(k string) string {
		if k == MaxLabelBytesEnv {
			return "100"
		}
		return ""
	})
	if tooBig.Translated() || len(tooBig.Manual) != 1 {
		t.Fatalf("label cap not applied: %+v", tooBig)
	}
}

func TestNodeMapParseAndResolve(t *testing.T) {
	m, err := ParseNodeMap("srv-a=edge-1, srv-b=n2")
	if err != nil || m["srv-a"] != "edge-1" || m["srv-b"] != "n2" {
		t.Fatalf("parse %v %v", m, err)
	}
	for _, bad := range []string{"novalue", "=x", "a=b,a=c", "a=b\r\nc"} {
		if _, err := ParseNodeMap(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
	nodes := []NodeRef{{ID: "n1", Name: "edge-1"}, {ID: "n2", Name: "edge-2"}}
	ids, err := ResolveNodeMap(m, nodes)
	if err != nil || ids["srv-a"] != "n1" || ids["srv-b"] != "n2" {
		t.Fatalf("resolve %v %v", ids, err)
	}
	if _, err := ResolveNodeMap(map[string]string{"srv": "ghost"}, nodes); err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("unknown node not refused: %v", err)
	}
}

func TestTranslateIngressRedirectScopeAndSecrets(t *testing.T) {
	labels := strings.Join([]string{
		`traefik.http.middlewares.other.redirectregex.regex=^https://old\.example\.test/.*`,
		"traefik.http.middlewares.other.redirectregex.replacement=https://new.example.test/",
		`traefik.http.middlewares.pat.redirectregex.regex=^https?://(www\.)?app\.example\.test/a`,
		"traefik.http.middlewares.pat.redirectregex.replacement=https://new.example.test/",
		`traefik.http.middlewares.own.redirectregex.regex=^https://app\.example\.test/b`,
		"traefik.http.middlewares.own.redirectregex.replacement=https://new.example.test/b",
		"traefik.http.middlewares.usr.redirectregex.regex=^/c",
		"traefik.http.middlewares.usr.redirectregex.replacement=https://fakeuser:fakepass@new.example.test/",
		"traefik.http.middlewares.qry.redirectregex.regex=^/d",
		"traefik.http.middlewares.qry.redirectregex.replacement=https://new.example.test/?token=fakeqrytoken",
		"traefik.http.middlewares.h.headers.customresponseheaders.X-Signature=a8f3k29dk3m4n5b6v7c8x9z0",
		"traefik.http.middlewares.h.headers.customrequestheaders.X-Upstream=amqp://fakeu:fakep@mq.example.test",
	}, "\n")
	p := TranslateIngress(IngressSource{Labels: labels, App: "web", Domain: "app.example.test"}, func(string) string { return "" })
	if len(p.Forwarders) != 1 || p.Forwarders[0].Match.Path != "^/b" {
		t.Fatalf("only the rule scoped to the app's own host may translate: %+v", p.Forwarders)
	}
	if len(p.Headers) != 0 {
		t.Fatalf("secret-looking header values copied: %+v", p.Headers)
	}
	out, _ := json.Marshal(p)
	for _, secret := range []string{"fakepass", "fakeqrytoken", "a8f3k29dk3m4n5b6v7c8x9z0", "fakep@"} {
		if strings.Contains(string(out), secret) {
			t.Fatalf("plan leaks %q: %s", secret, out)
		}
	}
}

func TestResolveNodeMapRefusesDuplicateNames(t *testing.T) {
	nodes := []NodeRef{{ID: "n1", Name: "edge"}, {ID: "n2", Name: "edge"}}
	if _, err := ResolveNodeMap(map[string]string{"srv": "edge"}, nodes); err == nil {
		t.Fatal("ambiguous node name accepted")
	}
	if ids, err := ResolveNodeMap(map[string]string{"srv": "n2"}, nodes); err != nil || ids["srv"] != "n2" {
		t.Fatalf("id lookup failed: %v %v", ids, err)
	}
}
