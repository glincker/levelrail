package dnszones

import (
	"fmt"
	"slices"
	"strings"
)

// TemplateParam is one input a template needs.
type TemplateParam struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

// Template is a named set of records for a common setup. Data only.
type Template struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Params      []TemplateParam `json:"params"`
	render      func(p map[string]string) []RecordSet
}

func txt(name, v string) RecordSet { return RecordSet{Name: name, Type: "TXT", Values: []string{v}} }

func dmarcValue(p map[string]string) string {
	v := "v=DMARC1; p=" + p["policy"]
	if p["report_email"] != "" {
		v += "; rua=mailto:" + p["report_email"]
	}
	return v
}

var dmarcParams = []TemplateParam{
	{Key: "policy", Label: "DMARC policy (none, quarantine, reject)", Default: "quarantine"},
	{Key: "report_email", Label: "DMARC report address", Placeholder: "dmarc@example.com"},
}

// Templates lists every built in template.
var Templates = []Template{
	{
		ID: "email", Name: "Email (any provider)", Description: "MX, SPF and DMARC for a mail host you run or rent.",
		Params: append([]TemplateParam{
			{Key: "mx_host", Label: "Mail server host", Required: true, Placeholder: "mail.example.com"},
			{Key: "spf", Label: "SPF mechanisms", Default: "mx", Placeholder: "mx include:_spf.example.net"},
		}, dmarcParams...),
		render: func(p map[string]string) []RecordSet {
			return []RecordSet{
				{Name: Apex, Type: "MX", Values: []string{"10 " + p["mx_host"]}},
				txt(Apex, "v=spf1 "+p["spf"]+" ~all"),
				txt("_dmarc", dmarcValue(p)),
			}
		},
	},
	{
		ID: "dkim", Name: "DKIM key", Description: "Publish a DKIM public key under a selector.",
		Params: []TemplateParam{
			{Key: "selector", Label: "Selector", Required: true, Placeholder: "default"},
			{Key: "public_key", Label: "Public key (base64, without headers)", Required: true},
		},
		render: func(p map[string]string) []RecordSet {
			return []RecordSet{txt(p["selector"]+"._domainkey", "v=DKIM1; k=rsa; p="+p["public_key"])}
		},
	},
	{
		ID: "verification", Name: "Verification TXT", Description: "A TXT token a service asks you to publish to prove ownership.",
		Params: []TemplateParam{
			{Key: "name", Label: "Record name", Default: Apex},
			{Key: "value", Label: "Token", Required: true},
		},
		render: func(p map[string]string) []RecordSet { return []RecordSet{txt(p["name"], p["value"])} },
	},
	{
		ID: "www-to-apex", Name: "www to apex", Description: "Point www at the zone apex with a CNAME.",
		render: func(map[string]string) []RecordSet {
			return []RecordSet{{Name: "www", Type: "CNAME", Values: []string{Apex}}}
		},
	},
	{
		ID: "google-workspace", Name: "Google Workspace", Description: "Google's MX and SPF, plus DMARC.",
		Params: dmarcParams,
		render: func(p map[string]string) []RecordSet {
			return []RecordSet{
				{Name: Apex, Type: "MX", Values: []string{"1 smtp.google.com"}},
				txt(Apex, "v=spf1 include:_spf.google.com ~all"),
				txt("_dmarc", dmarcValue(p)),
			}
		},
	},
	{
		ID: "microsoft-365", Name: "Microsoft 365", Description: "Exchange Online MX, SPF and autodiscover, plus DMARC.",
		Params: append([]TemplateParam{
			{Key: "mx_prefix", Label: "MX prefix (your domain with dots as dashes)", Required: true, Placeholder: "example-com"},
		}, dmarcParams...),
		render: func(p map[string]string) []RecordSet {
			return []RecordSet{
				{Name: Apex, Type: "MX", Values: []string{"0 " + p["mx_prefix"] + ".mail.protection.outlook.com"}},
				txt(Apex, "v=spf1 include:spf.protection.outlook.com -all"),
				{Name: "autodiscover", Type: "CNAME", Values: []string{"autodiscover.outlook.com"}},
				txt("_dmarc", dmarcValue(p)),
			}
		},
	},
	{
		ID: "caa-letsencrypt", Name: "CAA for Let's Encrypt", Description: "Allow only Let's Encrypt to issue certificates, wildcards included.",
		render: func(map[string]string) []RecordSet {
			return []RecordSet{{Name: Apex, Type: "CAA", Values: []string{`0 issue "letsencrypt.org"`, `0 issuewild "letsencrypt.org"`}}}
		},
	},
}

// FindTemplate returns the template with id.
func FindTemplate(id string) (Template, bool) {
	i := slices.IndexFunc(Templates, func(t Template) bool { return t.ID == id })
	if i < 0 {
		return Template{}, false
	}
	return Templates[i], true
}

// Render fills defaults, checks required params and returns the template's sets.
func (t Template) Render(params map[string]string) ([]RecordSet, error) {
	p := make(map[string]string, len(t.Params))
	for _, tp := range t.Params {
		v := strings.TrimSpace(params[tp.Key])
		if v == "" {
			v = tp.Default
		}
		if v == "" && tp.Required {
			return nil, fmt.Errorf("dnszones: template %s: %s is required", t.ID, tp.Key)
		}
		p[tp.Key] = v
	}
	return t.render(p), nil
}

// MergeTemplate layers template sets over existing ones. TXT sets keep the
// other values already published at that name (verification tokens), except
// a second SPF record, which would make SPF fail.
func MergeTemplate(existing, sets []RecordSet) []RecordSet {
	out := make([]RecordSet, 0, len(sets))
	for _, s := range sets {
		i := slices.IndexFunc(existing, func(e RecordSet) bool { return e.Key() == s.Key() })
		if s.Type != "TXT" || i < 0 {
			if i >= 0 {
				s.TTL = existing[i].TTL
			}
			out = append(out, s)
			continue
		}
		merged := slices.Clone(s.Values)
		newSPF := slices.ContainsFunc(s.Values, isSPF)
		newDMARC := slices.ContainsFunc(s.Values, isDMARC)
		for _, v := range existing[i].Values {
			if (newSPF && isSPF(v)) || (newDMARC && isDMARC(v)) || slices.Contains(merged, v) {
				continue
			}
			merged = append(merged, v)
		}
		s.Values = merged
		s.TTL = existing[i].TTL
		out = append(out, s)
	}
	return out
}

func isSPF(v string) bool   { return strings.HasPrefix(strings.ToLower(v), "v=spf1") }
func isDMARC(v string) bool { return strings.HasPrefix(strings.ToLower(v), "v=dmarc1") }
