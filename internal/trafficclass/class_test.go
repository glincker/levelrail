package trafficclass

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	chromeUA  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
	safari15  = "Mozilla/5.0 (iPhone; CPU iPhone OS 15_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.6 Mobile/15E148 Safari/604.1"
	firefoxUA = "Mozilla/5.0 (X11; Linux x86_64; rv:127.0) Gecko/20100101 Firefox/127.0"
)

func req(method, path, ua string, h map[string]string) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	if ua != "" {
		r.Header.Set("User-Agent", ua)
	} else {
		r.Header.Del("User-Agent")
	}
	for k, v := range h {
		r.Header.Set(k, v)
	}
	return r
}

func TestClassify(t *testing.T) {
	html := Response{Status: 200, ContentType: "text/html; charset=utf-8"}
	jsonResp := Response{Status: 200, ContentType: "application/json"}
	cases := []struct {
		name     string
		r        *http.Request
		resp     Response
		rules    *Rules
		want     Class
		soft     bool
		bot      string
		wantRule int
	}{
		{"chrome document", req("GET", "/pricing", chromeUA, map[string]string{"Sec-Fetch-Dest": "document"}), html, nil, Page, false, "", 0},
		{"firefox document", req("GET", "/", firefoxUA, map[string]string{"Sec-Fetch-Dest": "document"}), html, nil, Page, false, "", 0},
		{"xhr dest empty", req("GET", "/dash/poll", chromeUA, map[string]string{"Sec-Fetch-Dest": "empty"}), jsonResp, nil, API, false, "", 0},
		{"xhr dest empty html response is not a page", req("GET", "/partial", chromeUA, map[string]string{"Sec-Fetch-Dest": "empty"}), html, nil, API, false, "", 0},
		{"script dest", req("GET", "/main.js", chromeUA, map[string]string{"Sec-Fetch-Dest": "script"}), Response{Status: 200, ContentType: "text/javascript"}, nil, Asset, false, "", 0},
		{"image dest", req("GET", "/hero", chromeUA, map[string]string{"Sec-Fetch-Dest": "image"}), Response{Status: 200}, nil, Asset, false, "", 0},
		{"iframe dest", req("GET", "/embed", chromeUA, map[string]string{"Sec-Fetch-Dest": "iframe"}), html, nil, Asset, false, "", 0},
		{"unknown dest", req("GET", "/x", chromeUA, map[string]string{"Sec-Fetch-Dest": "weird"}), html, nil, Other, false, "", 0},
		{"googlebot", req("GET", "/", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", map[string]string{"Sec-Fetch-Dest": "document"}), html, nil, Bot, false, "googlebot", 0},
		{"gptbot", req("GET", "/blog", "Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; GPTBot/1.1; +https://openai.com/gptbot)", nil), html, nil, Bot, false, "gptbot", 0},
		{"headless chrome sends document", req("GET", "/", "Mozilla/5.0 HeadlessChrome/126.0.0.0 Safari/537.36", map[string]string{"Sec-Fetch-Dest": "document"}), html, nil, Bot, false, "headlesschrome", 0},
		{"empty user agent", req("GET", "/", "", nil), html, nil, Bot, false, UnknownBot, 0},
		{"curl", req("GET", "/", "curl/8.5.0", map[string]string{"Accept": "*/*"}), html, nil, Bot, false, "curl/", 0},
		{"shielded real browser", req("GET", "/", chromeUA, nil), Response{Status: 429, Shielded: true}, nil, Bot, false, UnknownBot, 0},
		{"sec-purpose prefetch", req("GET", "/next", chromeUA, map[string]string{"Sec-Purpose": "prefetch;prerender", "Sec-Fetch-Dest": "document"}), html, nil, Prefetch, false, "", 0},
		{"purpose prefetch", req("GET", "/next", chromeUA, map[string]string{"Purpose": "prefetch"}), html, nil, Prefetch, false, "", 0},
		{"x-moz prefetch", req("GET", "/next", firefoxUA, map[string]string{"X-Moz": "prefetch"}), html, nil, Prefetch, false, "", 0},
		{"next soft navigation", req("GET", "/blog/post", chromeUA, map[string]string{"RSC": "1", "Sec-Fetch-Dest": "empty"}), Response{Status: 200, ContentType: "text/x-component"}, nil, Page, true, "", 0},
		{"next router prefetch", req("GET", "/blog/post", chromeUA, map[string]string{"RSC": "1", "Next-Router-Prefetch": "1", "Sec-Fetch-Dest": "empty"}), Response{Status: 200}, nil, Prefetch, false, "", 0},
		{"next data path is api", req("GET", "/_next/data/abc/blog.json", chromeUA, map[string]string{"Sec-Fetch-Dest": "empty"}), jsonResp, nil, API, false, "", 4},
		{"safari 15 document no sec-fetch", req("GET", "/about", safari15, map[string]string{"Accept": "text/html,application/xhtml+xml,*/*;q=0.8"}), html, nil, Page, false, "", 0},
		{"old browser json", req("GET", "/data", safari15, map[string]string{"Accept": "application/json"}), jsonResp, nil, API, false, "", 0},
		{"old browser xhr header", req("POST", "/rpc", safari15, map[string]string{"X-Requested-With": "XMLHttpRequest"}), Response{Status: 200, ContentType: "text/plain"}, nil, API, false, "", 0},
		{"old browser event stream", req("GET", "/events", safari15, map[string]string{"Accept": "text/event-stream"}), Response{Status: 200, ContentType: "text/event-stream"}, nil, API, false, "", 0},
		{"old browser extension", req("GET", "/logo.PNG", safari15, nil), Response{Status: 200, ContentType: "image/png"}, nil, Asset, false, "", 0},
		{"old browser html without accept", req("GET", "/x", safari15, nil), html, nil, Other, false, "", 0},
		{"head request fallback", req("HEAD", "/x", safari15, map[string]string{"Accept": "text/html"}), html, nil, Other, false, "", 0},
		{"api path rule beats document header", req("GET", "/api/v1/apps", chromeUA, map[string]string{"Sec-Fetch-Dest": "document"}), html, nil, API, false, "", 1},
		{"asset path rule beats empty dest", req("GET", "/assets/app.js", chromeUA, map[string]string{"Sec-Fetch-Dest": "empty"}), Response{Status: 200}, nil, Asset, false, "", 7},
		{"path rule beats bot ua", req("GET", "/robots.txt", "Googlebot/2.1", nil), Response{Status: 200}, nil, Asset, false, "", 11},
		{"sitemap glob", req("GET", "/sitemap-0.xml", chromeUA, nil), Response{Status: 200}, nil, Asset, false, "", 12},
		{"well-known", req("GET", "/.well-known/security.txt", chromeUA, nil), Response{Status: 200}, nil, Asset, false, "", 14},
		{"custom rule first", req("GET", "/api/page", chromeUA, nil), html, mustNewRules(t, Rule{"/api/page", Page}, Rule{"/api/*", API}), Page, false, "", 1},
		{"custom rule ignore", req("GET", "/healthz", "kube-probe/1.29", nil), Response{Status: 200}, mustNewRules(t, Rule{"/healthz", Ignored}), Ignored, false, "", 1},
		{"custom rules replace defaults", req("GET", "/api/x", chromeUA, map[string]string{"Sec-Fetch-Dest": "document"}), html, mustNewRules(t, Rule{"/blog/*", Page}), Page, false, "", 0},
		{"plain browser navigation without headers is other", req("GET", "/x", chromeUA, nil), Response{Status: 200}, nil, Other, false, "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.r, tc.resp, tc.rules)
			if got.Class != tc.want || got.Soft != tc.soft || got.Bot != tc.bot || got.Rule != tc.wantRule {
				t.Fatalf("Classify = %+v, want class %s soft %v bot %q rule %d", got, tc.want, tc.soft, tc.bot, tc.wantRule)
			}
		})
	}
}

func mustNewRules(t *testing.T, rs ...Rule) *Rules {
	t.Helper()
	r, err := NewRules(rs)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNewRulesValidation(t *testing.T) {
	t.Setenv("APP_TRAFFIC_RULES_MAX", "2")
	for name, rs := range map[string][]Rule{
		"no leading slash": {{"api/*", API}},
		"bad class":        {{"/x", Bot}},
		"over the cap":     {{"/a", API}, {"/b", API}, {"/c", API}},
	} {
		if _, err := NewRules(rs); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if _, err := NewRules([]Rule{{"/a", API}, {"/b", Page}}); err != nil {
		t.Fatalf("valid rules rejected: %v", err)
	}
}

func TestGlobMatch(t *testing.T) {
	for _, tc := range []struct {
		pat, s string
		want   bool
	}{
		{"/api/*", "/api/v1/x", true},
		{"/api/*", "/api/", true},
		{"/api/*", "/api", false},
		{"/graphql", "/graphql", true},
		{"/graphql", "/graphql/x", false},
		{"/sitemap*.xml", "/sitemap.xml", true},
		{"/sitemap*.xml", "/sitemap-1.xml", true},
		{"/sitemap*.xml", "/sitemap.xml.gz", false},
		{"/a*b*c", "/aXXbYYc", true},
		{"/a*b*c", "/aXXbYY", false},
		{"/*", "/", true},
	} {
		if got := globMatch(tc.pat, tc.s); got != tc.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tc.pat, tc.s, got, tc.want)
		}
	}
}

func TestIsPageView(t *testing.T) {
	for _, tc := range []struct {
		method string
		status int
		want   bool
	}{
		{"GET", 200, true}, {"GET", 204, true}, {"GET", 301, false}, {"GET", 304, false},
		{"GET", 404, false}, {"GET", 500, false}, {"POST", 200, false}, {"HEAD", 200, false},
	} {
		if got := IsPageView(tc.method, tc.status); got != tc.want {
			t.Errorf("IsPageView(%s, %d) = %v, want %v", tc.method, tc.status, got, tc.want)
		}
	}
}

func TestBrowserUserAgentsAreNotBots(t *testing.T) {
	for _, ua := range []string{chromeUA, safari15, firefoxUA,
		"Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 Edg/126.0.0.0",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/126.0.0.0 Mobile/15E148 Safari/604.1",
	} {
		if name, ok := botName(ua, false); ok {
			t.Errorf("%q flagged as bot %q", ua, name)
		}
	}
}

func BenchmarkClassify(b *testing.B) {
	r := req("GET", "/dash/poll", chromeUA, map[string]string{"Sec-Fetch-Dest": "empty"})
	resp := Response{Status: 200, ContentType: "application/json"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Classify(r, resp, nil)
	}
}

func TestClassifyDoesNotAllocate(t *testing.T) {
	r := req("GET", "/dash/poll", chromeUA, map[string]string{"Sec-Fetch-Dest": "empty"})
	if n := testing.AllocsPerRun(100, func() { Classify(r, Response{Status: 200}, nil) }); n != 0 {
		t.Fatalf("Classify allocated %v times per run, want 0", n)
	}
}
