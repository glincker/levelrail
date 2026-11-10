package api

import "testing"

func TestPreviewHostTemplateRender(t *testing.T) {
	host, ok := renderPreviewHost("pr-{number}.{domain}", "example.com", 12, "feat/x")
	if !ok || host != "pr-12.example.com" {
		t.Fatalf("got %q %v, want pr-12.example.com", host, ok)
	}
	host, ok = renderPreviewHost("{branch}.{domain}", "example.com", 1, "Feat/My_Thing")
	if !ok || host == "" {
		t.Fatalf("branch template did not render: %q %v", host, ok)
	}
}

func TestPreviewHostTemplateValidate(t *testing.T) {
	owned := []string{"example.com"}
	cases := []struct {
		name, tpl, wild string
		ok              bool
	}{
		{"valid", "pr-{number}.{domain}", "example.com", true},
		{"valid subdomain", "{branch}.{domain}", "preview.example.com", true},
		{"no placeholder", "preview.{domain}", "example.com", false},
		{"foreign domain", "pr-{number}.{domain}", "evil.org", false},
		{"suffix lookalike", "pr-{number}.{domain}", "badexample.com", false},
		{"literal foreign host", "pr-{number}.evil.org", "example.com", false},
		{"port", "pr-{number}.{domain}:8080", "example.com", false},
		{"empty clears", "", "", true},
	}
	for _, c := range cases {
		if msg := validatePreviewHostTemplate(c.tpl, c.wild, owned); (msg == "") != c.ok {
			t.Errorf("%s: msg=%q, want ok=%v", c.name, msg, c.ok)
		}
	}
}
