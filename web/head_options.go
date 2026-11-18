package web

import (
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/brand"
)

// HeadOptions overrides the shell's share tags. The zero value changes
// nothing, so normal instances render exactly as before.
type HeadOptions struct {
	Title       string
	Description string
	URL         string
	ImageURL    string
	ImageAlt    string
	ImageWidth  int
	ImageHeight int
	// CSP replaces DashboardCSP on every response when set (demo analytics).
	CSP string
	// GTMID, when set, adds the server-rendered Google Tag Manager loader to
	// the head for requests that do not send Do Not Track or Sec-GPC.
	GTMID string
}

func (o HeadOptions) isZero() bool {
	o.CSP = ""
	return o == HeadOptions{}
}

// injectShareHead is the HeadOptions variant of the brand head: same tag
// set plus canonical, og:url and a large twitter card.
func injectShareHead(page []byte, b *brand.Brand, o HeadOptions, withGTM bool) []byte {
	e := html.EscapeString
	title := o.Title
	if title == "" {
		title = b.Name
	}
	desc := o.Description
	if desc == "" {
		desc = neutralDescription
	}
	tags := []string{
		`<meta name="description" content="` + e(desc) + `" />`,
		`<meta name="theme-color" content="` + e(b.PrimaryColor) + `" />`,
		`<meta property="og:site_name" content="` + e(b.Name) + `" />`,
		`<meta property="og:title" content="` + e(title) + `" />`,
		`<meta property="og:description" content="` + e(desc) + `" />`,
	}
	if o.URL != "" {
		tags = append(tags,
			`<link rel="canonical" href="`+e(o.URL)+`" />`,
			`<meta property="og:url" content="`+e(o.URL)+`" />`)
	}
	tags = append(tags, `<meta name="twitter:card" content="`+twitterCard(o)+`" />`,
		`<meta name="twitter:title" content="`+e(title)+`" />`,
		`<meta name="twitter:description" content="`+e(desc)+`" />`)
	if o.ImageURL != "" {
		tags = append(tags,
			`<meta property="og:image" content="`+e(o.ImageURL)+`" />`,
			`<meta name="twitter:image" content="`+e(o.ImageURL)+`" />`)
		if o.ImageWidth > 0 && o.ImageHeight > 0 {
			tags = append(tags,
				`<meta property="og:image:width" content="`+strconv.Itoa(o.ImageWidth)+`" />`,
				`<meta property="og:image:height" content="`+strconv.Itoa(o.ImageHeight)+`" />`)
		}
		if o.ImageAlt != "" {
			tags = append(tags, `<meta property="og:image:alt" content="`+e(o.ImageAlt)+`" />`)
		}
	}
	if withGTM && o.GTMID != "" {
		tags = append(tags, gtmHeadTags(o.GTMID)...)
	}
	out := titleTag.ReplaceAllLiteral(page, []byte("<title>"+e(title)+"</title>"))
	out = metaTags.ReplaceAll(out, nil)
	meta := strings.Join(tags, "\n    ")
	return []byte(strings.Replace(string(out), "</head>", "    "+meta+"\n  </head>", 1))
}

func twitterCard(o HeadOptions) string {
	if o.ImageURL != "" && o.ImageWidth >= 600 {
		return "summary_large_image"
	}
	return "summary"
}

// GTMInitPath is the same-origin script that seeds dataLayer before gtm.js runs.
const GTMInitPath = "/gtm-init.js"

// gtmHeadTags returns the external-only GTM loader: no inline script, so the CSP
// keeps script-src without unsafe-inline. The init script is a plain blocking
// script so gtm.start is queued before the async gtm.js executes.
func gtmHeadTags(id string) []string {
	return []string{
		`<script src="` + GTMInitPath + `"></script>`,
		`<script async src="https://www.googletagmanager.com/gtm.js?id=` + url.QueryEscape(id) + `"></script>`,
	}
}

// GTMInitScript is the body served at GTMInitPath.
const GTMInitScript = "window.dataLayer = window.dataLayer || [];\nwindow.dataLayer.push({ 'gtm.start': Date.now(), event: 'gtm.js' });\n"

// optedOutOfTracking reports Do Not Track or Global Privacy Control request headers.
func optedOutOfTracking(r *http.Request) bool {
	return r.Header.Get("DNT") == "1" || r.Header.Get("Sec-GPC") == "1"
}
