package mailtemplates

import (
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	errEmptySubject = errors.New("subject is required")
	errTooLong      = errors.New("subject, preheader or body is too long")
)

// Branding is the identity the layout wears, filled from the brand layer.
type Branding struct {
	Name             string
	PrimaryColor     string
	PrimaryColorDark string
	SupportEmail     string
	DashboardURL     string
	InstanceName     string
}

// Layout is the operator-editable frame around every event template.
type Layout struct {
	HeaderImageURL string `json:"header_image_url"`
	FooterText     string `json:"footer_text"`
}

// Options carries per-message values the layout needs.
type Options struct {
	// UnsubscribeURL, when set, adds the unsubscribe line to the footer.
	UnsubscribeURL string
}

// Rendered is a finished message.
type Rendered struct {
	Subject   string `json:"subject"`
	Preheader string `json:"preheader"`
	Text      string `json:"text"`
	HTML      string `json:"html"`
}

var colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$`)

// ValidateLayout checks the header image is an https URL and the footer
// only uses global tokens.
func ValidateLayout(l Layout) error {
	if l.HeaderImageURL != "" {
		u, err := url.Parse(l.HeaderImageURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || len(l.HeaderImageURL) > 500 {
			return errors.New("header image must be an https URL")
		}
	}
	if len(l.FooterText) > 1000 {
		return errors.New("footer text is too long")
	}
	return ValidateTokens(l.FooterText, tokensByScope(ScopeGlobal))
}

// GlobalValues returns the global-scope token values for b.
func GlobalValues(b Branding, now time.Time) map[string]string {
	instance := b.InstanceName
	if instance == "" {
		instance = b.Name
	}
	return map[string]string{
		"brand_name":    b.Name,
		"support_email": b.SupportEmail,
		"dashboard_url": b.DashboardURL,
		"instance_name": instance,
		"year":          strconv.Itoa(now.Year()),
	}
}

func merged(b Branding, values map[string]string) map[string]string {
	out := GlobalValues(b, time.Now())
	for k, v := range values {
		out[k] = v
	}
	return out
}

func accent(c, fallback string) string {
	if colorRe.MatchString(c) {
		return c
	}
	return fallback
}

// Render produces the subject, plain text and HTML for t inside the shared
// layout. Values are escaped for HTML, so a value can never add markup.
func Render(b Branding, l Layout, t Template, values map[string]string, o Options) Rendered {
	vals := merged(b, values)
	subject := strings.TrimSpace(SubstituteText(t.Subject, vals))
	preheader := strings.TrimSpace(SubstituteText(t.Preheader, vals))
	blocks := parseBlocks(t.Body)
	light := accent(b.PrimaryColor, "#2563eb")
	dark := accent(b.PrimaryColorDark, light)

	var h, tx strings.Builder
	for _, bl := range blocks {
		switch bl.kind {
		case blockHeading:
			h.WriteString(`<h1 class="ink" style="margin:0 0 16px;font-size:22px;line-height:1.3;color:#111827">` + inlineHTML(bl.lines[0], vals) + `</h1>`)
			tx.WriteString(inlineText(bl.lines[0], vals) + "\n\n")
		case blockList:
			h.WriteString(`<ul class="ink" style="margin:0 0 16px;padding-left:20px;color:#111827">`)
			for _, ln := range bl.lines {
				h.WriteString(`<li style="margin:0 0 6px">` + inlineHTML(ln, vals) + `</li>`)
				tx.WriteString("- " + inlineText(ln, vals) + "\n")
			}
			h.WriteString(`</ul>`)
			tx.WriteString("\n")
		case blockCallout:
			var parts []string
			for _, ln := range bl.lines {
				parts = append(parts, inlineHTML(ln, vals))
				tx.WriteString("> " + inlineText(ln, vals) + "\n")
			}
			tx.WriteString("\n")
			h.WriteString(`<div class="callout ink" style="margin:0 0 16px;padding:12px 16px;border-left:4px solid ` + light + `;background:#f3f4f6;color:#111827">` + strings.Join(parts, "<br>") + `</div>`)
		case blockButton:
			label, target, _ := strings.Cut(bl.lines[0], "|")
			href := safeURL(substitute(target, vals, false))
			h.WriteString(`<p style="margin:0 0 20px"><a class="btn" href="` + html.EscapeString(href) + `" style="display:inline-block;padding:12px 20px;background:` + light + `;color:#ffffff;text-decoration:none;border-radius:6px;font-weight:600">` + plainInline(label, vals) + `</a></p>`)
			tx.WriteString(inlineText(label, vals) + ": " + href + "\n\n")
		default:
			var parts []string
			for _, ln := range bl.lines {
				parts = append(parts, inlineHTML(ln, vals))
				tx.WriteString(inlineText(ln, vals) + "\n")
			}
			tx.WriteString("\n")
			h.WriteString(`<p class="ink" style="margin:0 0 16px;line-height:1.6;color:#111827">` + strings.Join(parts, "<br>") + `</p>`)
		}
	}

	footer := strings.ReplaceAll(strings.TrimSpace(substitute(html.EscapeString(l.FooterText), vals, true)), "\n", "<br>")
	footerText := strings.TrimSpace(SubstituteText(l.FooterText, vals))
	unsub, unsubText := "", ""
	if o.UnsubscribeURL != "" {
		target := html.EscapeString(safeURL(o.UnsubscribeURL))
		unsub = `<p style="margin:8px 0 0"><a class="lnk" href="` + target + `" style="color:#6b7280;text-decoration:underline">Unsubscribe from these messages</a></p>`
		unsubText = "\nUnsubscribe from these messages: " + o.UnsubscribeURL + "\n"
	}

	header := `<span class="ink" style="font-size:18px;font-weight:700;color:#111827">` + html.EscapeString(b.Name) + `</span>`
	if l.HeaderImageURL != "" {
		header = `<img src="` + html.EscapeString(l.HeaderImageURL) + `" alt="` + html.EscapeString(b.Name) + `" style="display:block;max-width:100%;height:auto;border:0;margin:0 0 8px">` + header
	}

	page := fmt.Sprintf(pageTemplate, html.EscapeString(subject), dark, html.EscapeString(preheader), light, header, h.String(), footer, unsub)
	page = strings.ReplaceAll(page, "var(--mail-accent,#2563eb)", light)

	text := strings.TrimSpace(tx.String())
	if footerText != "" {
		text += "\n\n--\n" + footerText
	}
	text += "\n" + unsubText
	return Rendered{Subject: subject, Preheader: preheader, Text: text, HTML: page}
}

const pageTemplate = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light dark"><meta name="supported-color-schemes" content="light dark">
<title>%s</title>
<style>
@media (max-width:620px){.wrap{width:100%%!important}.pad{padding:20px!important}}
@media (prefers-color-scheme:dark){
 body,.bg{background:#0b0f14!important}
 .card{background:#111821!important}
 .ink,.ink *{color:#e5e7eb!important}
 .callout{background:#1a2430!important}
 .btn{background:%s!important}
 .muted,.muted *{color:#9ca3af!important}
}
</style></head>
<body class="bg" style="margin:0;padding:0;background:#f3f4f6">
<div style="display:none;max-height:0;overflow:hidden;opacity:0">%s</div>
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" class="bg" style="background:#f3f4f6"><tr><td align="center" style="padding:24px 12px">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" class="wrap card" style="width:600px;max-width:100%%;background:#ffffff;border-radius:8px;border-top:4px solid %s;font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;font-size:15px">
<tr><td class="pad" style="padding:24px 32px 8px">%s</td></tr>
<tr><td class="pad" style="padding:8px 32px 24px">%s</td></tr>
<tr><td class="pad muted" style="padding:16px 32px 24px;border-top:1px solid #e5e7eb;font-size:12px;line-height:1.5;color:#6b7280">%s%s</td></tr>
</table></td></tr></table></body></html>`

// ForceScheme pins a rendered page to "light" or "dark" for previews; any
// other value leaves the page following the viewer's own setting.
func ForceScheme(page, scheme string) string {
	const dark = "@media (prefers-color-scheme:dark){"
	switch scheme {
	case "dark":
		return strings.Replace(page, dark, "@media all{", 1)
	case "light":
		return strings.Replace(page, dark, "@media not all{", 1)
	}
	return page
}
