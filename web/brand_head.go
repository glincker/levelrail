package web

import (
	"encoding/json"
	"html"
	"net/http"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/brand"
)

const (
	manifestPath       = "/manifest.webmanifest"
	neutralTitle       = "Dashboard"
	neutralDescription = "Self-hosted deployment platform"
	iconPath           = "/apple-touch-icon.png"
)

var (
	titleTag = regexp.MustCompile(`(?is)<title>.*?</title>`)
	metaTags = regexp.MustCompile(`(?i)[ \t]*<meta\s+(?:name="(?:description|theme-color|twitter:[^"]*)"|property="og:[^"]*")[^>]*>\n?`)
)

type manifestIcon struct {
	Src   string `json:"src"`
	Sizes string `json:"sizes"`
	Type  string `json:"type"`
}

type webManifest struct {
	Name            string         `json:"name"`
	ShortName       string         `json:"short_name"`
	Description     string         `json:"description"`
	StartURL        string         `json:"start_url"`
	Display         string         `json:"display"`
	ThemeColor      string         `json:"theme_color,omitempty"`
	BackgroundColor string         `json:"background_color"`
	Icons           []manifestIcon `json:"icons"`
}

func brandName(b *brand.Brand) string {
	if b == nil || b.Name == "" {
		return neutralTitle
	}
	return b.Name
}

func serveManifest(w http.ResponseWriter, b *brand.Brand) {
	m := webManifest{
		Name:            brandName(b),
		ShortName:       brandName(b),
		Description:     neutralDescription,
		StartURL:        "/",
		Display:         "standalone",
		BackgroundColor: "#ffffff",
		Icons: []manifestIcon{
			{Src: "/favicon.svg", Sizes: "any", Type: "image/svg+xml"},
			{Src: iconPath, Sizes: "180x180", Type: "image/png"},
		},
	}
	if b != nil {
		if b.ShortName != "" {
			m.ShortName = b.ShortName
		}
		m.ThemeColor = b.PrimaryColor
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-cache")
	_ = json.NewEncoder(w).Encode(m)
}

// injectBrandHead rewrites the static shell's title and appends brand meta
// tags so crawlers and link previews, which never run the SPA, see them.
func injectBrandHead(page []byte, b *brand.Brand, r *http.Request) []byte {
	if b == nil || b.Name == "" {
		return page
	}
	name := html.EscapeString(b.Name)
	img := html.EscapeString(requestOrigin(r) + iconPath)
	desc := html.EscapeString(neutralDescription)
	meta := strings.Join([]string{
		`<meta name="description" content="` + desc + `" />`,
		`<meta name="theme-color" content="` + html.EscapeString(b.PrimaryColor) + `" />`,
		`<meta property="og:site_name" content="` + name + `" />`,
		`<meta property="og:title" content="` + name + `" />`,
		`<meta property="og:description" content="` + desc + `" />`,
		`<meta property="og:image" content="` + img + `" />`,
		`<meta name="twitter:card" content="summary" />`,
		`<meta name="twitter:title" content="` + name + `" />`,
		`<meta name="twitter:description" content="` + desc + `" />`,
		`<meta name="twitter:image" content="` + img + `" />`,
	}, "\n    ")
	out := titleTag.ReplaceAllLiteral(page, []byte("<title>"+name+"</title>"))
	out = metaTags.ReplaceAll(out, nil)
	return []byte(strings.Replace(string(out), "</head>", "    "+meta+"\n  </head>", 1))
}

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
