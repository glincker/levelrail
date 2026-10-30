package mailrecords

import "sort"

// MXEntry is one MX target of a hosted mail preset.
type MXEntry struct {
	Priority int
	Host     string
}

// DKIMCNAME is a fixed-shape DKIM delegation; %s in Target is the domain.
type DKIMCNAME struct {
	Selector string
	Target   string
}

// Preset holds the public DNS values of an external mail provider. Only values
// the provider documents for every customer are listed; per-account tokens and
// keys always come from the provider's own dashboard.
type Preset struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Source      string      `json:"source"`
	MX          []MXEntry   `json:"mx,omitempty"`
	SPFInclude  string      `json:"spf_include"`
	DKIMCNAMEs  []DKIMCNAME `json:"dkim_cnames,omitempty"`
	SendingOnly bool        `json:"sending_only,omitempty"`
	Note        string      `json:"note"`
}

// Presets is the data table, keyed by id. Resend is left out on purpose: its
// SPF and return-path records live on a per-account send subdomain.
var Presets = map[string]Preset{
	"google": {
		ID: "google", Name: "Google Workspace", Source: "Google Workspace Admin Help, MX record values",
		MX: []MXEntry{
			{1, "aspmx.l.google.com"}, {5, "alt1.aspmx.l.google.com"}, {5, "alt2.aspmx.l.google.com"},
			{10, "alt3.aspmx.l.google.com"}, {10, "alt4.aspmx.l.google.com"},
		},
		SPFInclude: "_spf.google.com",
		Note:       "Generate the DKIM key in the Google admin console (selector google by default), then re-run with the key.",
	},
	"fastmail": {
		ID: "fastmail", Name: "Fastmail", Source: "Fastmail help, DNS settings for your domain",
		MX:         []MXEntry{{10, "in1-smtp.messagingengine.com"}, {20, "in2-smtp.messagingengine.com"}},
		SPFInclude: "spf.messagingengine.com",
		DKIMCNAMEs: []DKIMCNAME{
			{"fm1", "fm1.%s.dkim.fmhosted.com"}, {"fm2", "fm2.%s.dkim.fmhosted.com"}, {"fm3", "fm3.%s.dkim.fmhosted.com"},
		},
		Note: "Confirm the three DKIM CNAMEs against the values Fastmail shows for your domain before relying on them.",
	},
	"proton": {
		ID: "proton", Name: "Proton Mail", Source: "Proton Mail support, custom domain setup",
		MX:         []MXEntry{{10, "mail.protonmail.ch"}, {20, "mailsec.protonmail.ch"}},
		SPFInclude: "_spf.protonmail.ch",
		Note:       "Proton also needs a domain verification TXT and three DKIM CNAMEs that are unique to your account; copy them from the Proton dashboard.",
	},
	"zoho": {
		ID: "zoho", Name: "Zoho Mail (US data center)", Source: "Zoho Mail help, MX records for the .com data center",
		MX:         []MXEntry{{10, "mx.zoho.com"}, {20, "mx2.zoho.com"}, {50, "mx3.zoho.com"}},
		SPFInclude: "zoho.com",
		Note:       "Accounts in the EU or India data centers use different hosts (zoho.eu, zoho.in); this preset is the .com data center only. The DKIM key is generated in Zoho.",
	},
	"ses": {
		ID: "ses", Name: "Amazon SES (sending only)", Source: "AWS SES developer guide, SPF authentication",
		SPFInclude: "amazonses.com", SendingOnly: true,
		Note: "Publish the three DKIM CNAMEs and the custom MAIL FROM records shown in the SES console; they are unique to your identity.",
	},
	"mailgun": {
		ID: "mailgun", Name: "Mailgun (sending only)", Source: "Mailgun documentation, domain verification",
		SPFInclude: "mailgun.org", SendingOnly: true,
		Note: "Publish the DKIM TXT and tracking CNAME shown on the Mailgun domain page; the key is unique to your domain.",
	},
	"postmark": {
		ID: "postmark", Name: "Postmark (sending only)", Source: "Postmark support, SPF and the Return-Path domain",
		SPFInclude: "spf.mtasv.net", SendingOnly: true,
		Note: "Postmark prefers a Return-Path CNAME (pm-bounces) over an apex SPF include; add the DKIM TXT and Return-Path from your Postmark sender signature.",
	},
}

// PresetIDs lists the preset ids, sorted.
func PresetIDs() []string {
	out := make([]string, 0, len(Presets))
	for id := range Presets {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
