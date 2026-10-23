package mailtemplates

import (
	"strings"
	"testing"
	"time"
)

func TestEveryEventDefaultValidates(t *testing.T) {
	for _, e := range Events() {
		if err := ValidateTemplate(e, e.Default); err != nil {
			t.Errorf("%s default: %v", e.Key, err)
		}
		for _, name := range e.Tokens {
			if _, ok := LookupToken(name); !ok {
				t.Errorf("%s lists unregistered token %q", e.Key, name)
			}
		}
	}
}

func TestValidateTokens(t *testing.T) {
	allowed := []string{"brand_name", "reset_link"}
	cases := []struct {
		text string
		ok   bool
	}{
		{"Hi {{brand_name}} {{ reset_link }}", true},
		{"no tokens", true},
		{"{{unknown}}", false},
		{"{{brand_name", false},
		{"{{brand-name}}", false},
	}
	for _, c := range cases {
		if err := ValidateTokens(c.text, allowed); (err == nil) != c.ok {
			t.Errorf("%q: err=%v, want ok=%v", c.text, err, c.ok)
		}
	}
}

func TestRenderEscapesValuesAndBlocksUnsafeMarkup(t *testing.T) {
	b := Branding{Name: "Acme", PrimaryColor: "#112233"}
	tpl := Template{
		Subject: "Hi {{first_name}}",
		Body:    "<script>alert(1)</script> {{first_name}}\n\n[bad](javascript:alert(1)) [good]({{dashboard_url}})\n\n!button Go|{{reset_link}}",
	}
	out := Render(b, Layout{FooterText: "Acme, 1 Main St"}, tpl, map[string]string{
		"first_name": `<img src=x onerror=alert(1)>`, "dashboard_url": "https://example.com", "reset_link": "javascript:alert(2)",
	}, Options{UnsubscribeURL: "https://example.com/mail/unsubscribe?t=x"})

	for _, bad := range []string{"<script>", "<img src=x", `href="javascript:`} {
		if strings.Contains(out.HTML, bad) {
			t.Errorf("html contains %q", bad)
		}
	}
	if !strings.Contains(out.HTML, `href="https://example.com"`) || !strings.Contains(out.HTML, "Unsubscribe") {
		t.Error("expected the safe link and the unsubscribe line")
	}
	if strings.Contains(out.Subject, "\n") || !strings.Contains(out.Text, "Unsubscribe") || strings.Contains(out.Text, "<b>") {
		t.Errorf("bad plain parts: %q / %q", out.Subject, out.Text)
	}
}

func TestLayoutValidation(t *testing.T) {
	if err := ValidateLayout(Layout{HeaderImageURL: "http://insecure.example/x.png"}); err == nil {
		t.Error("http header image accepted")
	}
	if err := ValidateLayout(Layout{FooterText: "{{reset_link}}"}); err == nil {
		t.Error("event token accepted in footer")
	}
	if err := ValidateLayout(Layout{HeaderImageURL: "https://example.com/x.png", FooterText: "(c) {{year}} {{brand_name}}"}); err != nil {
		t.Errorf("valid layout rejected: %v", err)
	}
	if got := GlobalValues(Branding{Name: "A"}, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC))["year"]; got != "2030" {
		t.Errorf("year = %q", got)
	}
}
