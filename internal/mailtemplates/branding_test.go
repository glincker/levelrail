package mailtemplates

import (
	"flag"
	"os"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite testdata golden files")

func testBranding() EmailBranding {
	return EmailBranding{
		LogoURL: "https://cdn.example.com/logo.png", BrandColor: "#112233",
		FooterText: "(c) {{year}} {{brand_name}}", FooterAddress: "1 Main St\nSpringfield",
		FooterLinks:   []FooterLink{{Label: "Privacy", URL: "https://example.com/privacy"}, {Label: "Terms", URL: "https://example.com/terms"}},
		ShowPoweredBy: true,
	}
}

func TestValidateBranding_Accepts(t *testing.T) {
	if err := ValidateBranding(testBranding(), LimitsFromEnv()); err != nil {
		t.Fatalf("valid branding rejected: %v", err)
	}
	if err := ValidateBranding(EmailBranding{}, LimitsFromEnv()); err != nil {
		t.Fatalf("empty branding rejected: %v", err)
	}
}

func TestValidateBranding_Rejects(t *testing.T) {
	lim := LimitsFromEnv()
	long := "https://example.com/" + strings.Repeat("a", lim.MaxLogoURL)
	cases := map[string]EmailBranding{
		"javascript logo": {LogoURL: "javascript:alert(1)"},
		"data logo":       {LogoURL: "data:image/png;base64,AAAA"},
		"http logo":       {LogoURL: "http://example.com/x.png"},
		"userinfo logo":   {LogoURL: "https://user:pw@example.com/x.png"}, //nolint:gosec // credential-bearing URL is the rejected input
		"space in logo":   {LogoURL: "https://example.com/a b.png"},
		"crlf in logo":    {LogoURL: "https://example.com/a\r\nX: y"},
		"oversized logo":  {LogoURL: long},
		"bad color":       {BrandColor: "red"},
		"short color":     {BrandColor: "#fff"},
		"unknown var":     {FooterText: "hello {{secret_token}}"},
		"event var":       {FooterText: "{{reset_link}}"},
		"malformed var":   {FooterAddress: "{{ brand_name"},
		"javascript link": {FooterLinks: []FooterLink{{Label: "x", URL: "javascript:alert(1)"}}},
		"data link":       {FooterLinks: []FooterLink{{Label: "x", URL: "data:text/html,hi"}}},
		"http link":       {FooterLinks: []FooterLink{{Label: "x", URL: "http://example.com"}}},
		"empty link":      {FooterLinks: []FooterLink{{Label: "x"}}},
		"empty label":     {FooterLinks: []FooterLink{{URL: "https://example.com"}}},
		"newline label":   {FooterLinks: []FooterLink{{Label: "a\nb", URL: "https://example.com"}}},
	}
	for name, b := range cases {
		if err := ValidateBranding(b, lim); err == nil {
			t.Errorf("%s: accepted, want rejection", name)
		}
	}

	var many []FooterLink
	for i := 0; i <= lim.MaxFooterLinks; i++ {
		many = append(many, FooterLink{Label: "l", URL: "https://example.com"})
	}
	if err := ValidateBranding(EmailBranding{FooterLinks: many}, lim); err == nil {
		t.Error("too many footer links accepted")
	}
	if err := ValidateBranding(EmailBranding{FooterText: strings.Repeat("a", lim.MaxFooterText+1)}, lim); err == nil {
		t.Error("oversized footer text accepted")
	}
}

func TestLimitsFromEnv(t *testing.T) {
	t.Setenv(EnvMaxFooterLinks, "2")
	t.Setenv(EnvMaxLogoURL, "garbage")
	lim := LimitsFromEnv()
	if lim.MaxFooterLinks != 2 || lim.MaxLogoURL != 500 {
		t.Errorf("limits = %+v", lim)
	}
}

func TestVariableAllowlist(t *testing.T) {
	for _, tok := range Tokens() {
		if err := ValidateTokens("{{"+tok.Name+"}}", []string{tok.Name}); err != nil {
			t.Errorf("registered token %s rejected: %v", tok.Name, err)
		}
	}
	if err := ValidateTemplate(mustEvent(t, EventPasswordReset), Template{Subject: "hi {{nope}}", Body: "x"}); err == nil {
		t.Error("unknown variable in subject accepted")
	}
}

func mustEvent(t *testing.T, key string) Event {
	t.Helper()
	ev, ok := LookupEvent(key)
	if !ok {
		t.Fatalf("no event %s", key)
	}
	return ev
}

func TestRenderNoRecursiveExpansion(t *testing.T) {
	tpl := Template{Subject: "Hi {{first_name}}", Body: "Hello {{first_name}} {{display_name}}"}
	out := Render(Branding{Name: "Acme"}, Layout{FooterText: "{{brand_name}}"}, tpl, map[string]string{
		"first_name": "{{display_name}}", "display_name": "{{email}}", "email": "leak@example.com", "brand_name": "{{year}}",
	}, Options{})
	for _, s := range []string{out.Subject, out.Text, out.HTML} {
		if strings.Contains(s, "leak@example.com") {
			t.Errorf("value was expanded a second time: %q", s)
		}
	}
	if out.Subject != "Hi {{display_name}}" {
		t.Errorf("subject = %q, want the literal substituted value", out.Subject)
	}
}

func TestRenderEscapesScriptInEveryField(t *testing.T) {
	b := Branding{Name: `<script>alert("n")</script>`}
	l := testBranding().Layout()
	l.FooterAddress = "<script>alert(1)</script>"
	l.FooterLinks = []FooterLink{{Label: "<b>x</b>", URL: "https://example.com/?a=1&b=\"2\""}}
	out := Render(b, l, Template{Subject: "s", Body: "{{first_name}}"}, map[string]string{"first_name": "<script>alert(2)</script>"}, Options{})
	if strings.Contains(out.HTML, "<script") || strings.Contains(out.HTML, "<b>x") {
		t.Errorf("unescaped markup in html: %s", out.HTML)
	}
	if !strings.Contains(out.HTML, "&lt;script&gt;") {
		t.Error("expected escaped script text")
	}
}

func TestRenderStripsCRLFFromHeaderValues(t *testing.T) {
	tpl := Template{Subject: "Hello {{display_name}}", Preheader: "p {{display_name}}", Body: "{{display_name}}"}
	out := Render(Branding{Name: "Acme"}, Layout{}, tpl, map[string]string{"display_name": "Sam\r\nBcc: evil@example.com"}, Options{})
	if strings.ContainsAny(out.Subject, "\r\n") || strings.ContainsAny(out.Preheader, "\r\n") {
		t.Errorf("CR or LF survived in subject %q / preheader %q", out.Subject, out.Preheader)
	}
	if strings.Contains(out.Text, "Sam\r") || strings.Contains(out.Text, "Sam\nBcc") {
		t.Errorf("CR or LF survived in plain text value: %q", out.Text)
	}
}

func TestRenderSkipsUnsafeFooterLinks(t *testing.T) {
	l := Layout{FooterLinks: []FooterLink{{Label: "bad", URL: "javascript:alert(1)"}, {Label: "ok", URL: "https://example.com/ok"}}}
	out := Render(Branding{Name: "Acme"}, l, Template{Subject: "s", Body: "b"}, nil, Options{})
	if strings.Contains(out.HTML, "javascript:") || strings.Contains(out.Text, "javascript:") {
		t.Error("javascript link rendered")
	}
	if !strings.Contains(out.HTML, `href="https://example.com/ok"`) {
		t.Error("safe link missing")
	}
}

func TestPoweredByOnlyWhenEnabled(t *testing.T) {
	on := Render(Branding{Name: "Acme"}, Layout{ShowPoweredBy: true}, Template{Subject: "s", Body: "b"}, nil, Options{})
	off := Render(Branding{Name: "Acme"}, Layout{}, Template{Subject: "s", Body: "b"}, nil, Options{})
	if !strings.Contains(on.HTML, "Powered by Acme") || !strings.Contains(on.Text, "Powered by Acme") {
		t.Error("powered-by missing when enabled")
	}
	if strings.Contains(off.HTML, "Powered by") {
		t.Error("powered-by shown when disabled")
	}
}

func TestGoldenBrandedRender(t *testing.T) {
	ev := mustEvent(t, EventPasswordReset)
	vals := SampleValues()
	vals["year"] = "2026"
	out := Render(Branding{Name: "Acme", PrimaryColor: "#112233"}, testBranding().Layout(), ev.Default, vals, Options{})
	got := out.HTML + "\n=====\n" + out.Text
	path := "testdata/branded_password_reset.golden"
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path) //nolint:gosec // fixed test fixture path
	if err != nil {
		t.Fatalf("read golden (run with -update-golden): %v", err)
	}
	if string(want) != got {
		t.Errorf("render differs from %s (run with -update-golden to refresh)", path)
	}
}
