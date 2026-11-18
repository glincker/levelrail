package web

import (
	"html"
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
}

func (o HeadOptions) isZero() bool { return o == HeadOptions{} }

// injectShareHead is the HeadOptions variant of the brand head: same tag
// set plus canonical, og:url and a large twitter card.
func injectShareHead(page []byte, b *brand.Brand, o HeadOptions) []byte {
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
