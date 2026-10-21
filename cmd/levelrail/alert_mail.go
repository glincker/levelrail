package main

import (
	"context"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/mailtemplates"
	"github.com/GLINCKER/levelrail/internal/store"
)

// alertMailRenderer renders alert and deploy emails through the operator-editable templates; any miss falls back to plain text.
func alertMailRenderer(db *store.DB, b brand.Brand) alerting.MailRenderFunc {
	return func(ctx context.Context, key, to string, values map[string]string) (email.Message, bool) {
		ev, ok := mailtemplates.LookupEvent(key)
		if !ok {
			return email.Message{}, false
		}
		tpl := ev.Default
		if t, err := db.GetMailTemplate(ctx, key); err == nil {
			cand := mailtemplates.Template{Subject: t.Subject, Preheader: t.Preheader, Body: t.Body}
			if mailtemplates.ValidateTemplate(ev, cand) == nil {
				tpl = cand
			}
		}
		var layout mailtemplates.Layout
		if l, err := db.GetMailLayout(ctx); err == nil {
			layout = mailtemplates.Layout{HeaderImageURL: l.HeaderImageURL, FooterText: l.FooterText}
		}
		base, _ := db.GetDashboardURL(ctx)
		instance := b.Name
		if u, err := url.Parse(base); err == nil && u.Host != "" {
			instance = u.Host
		}
		branding := mailtemplates.Branding{
			Name: b.Name, PrimaryColor: b.PrimaryColor, PrimaryColorDark: b.PrimaryColorDark,
			SupportEmail: b.SupportEmail, DashboardURL: base, InstanceName: instance,
		}
		vals := map[string]string{"email": to, "display_name": to, "first_name": "there"}
		for k, v := range values {
			vals[k] = v
		}
		if vals["deploy_url"] == "" {
			vals["deploy_url"] = base
		}
		out := mailtemplates.Render(branding, layout, tpl, vals, mailtemplates.Options{})
		return email.Message{To: to, Subject: out.Subject, Text: out.Text, HTML: out.HTML}, true
	}
}
